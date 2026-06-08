"""OKB Web 用的一次性 chat-turn 脚本（绕过 OpenKB 的 TTY REPL）。

通过 stdin 接收 JSON：{"action": "send", "kb_dir": "...", "session_id": "..." | null, "message": "..."}
- action=send: 加载或新建 session，跑一轮推理（调底层 Runner.run，非流式），保存 session，stdout 输出 JSON
- action=list: 列出 kb_dir 下所有 chat session（id/title/turns/updated_at）
- action=delete: 按 id 或前缀删除
- action=load: 加载某 session 的完整消息历史（用于前端恢复对话）

输出格式（JSON）：
- send 成功: {"ok": true, "session_id": "...", "answer": "...", "title": "..."}
- 失败:     {"ok": false, "error": "..."}
"""
from __future__ import annotations

import asyncio
import json
import sys
import traceback
from pathlib import Path
from typing import Any


def _load_kb_config(kb_dir: Path) -> dict[str, Any]:
    """Mirror OpenKB's _setup_llm_key + load_config minimal behavior."""
    from openkb.config import load_config

    cfg_path = kb_dir / ".openkb" / "config.yaml"
    return load_config(cfg_path) or {}


def _setup_env(kb_dir: Path) -> None:
    """直接复用 OpenKB CLI 的 _setup_llm_key 逻辑，确保 provider 路由、
    OPENAI_API_KEY/OPENAI_BASE_URL/DEEPSEEK_API_KEY 等所有需要的环境变量都设上。
    """
    # noqa: import 放函数内是为了等 _setup_env 真正被调时再去触碰 OpenKB
    from openkb.cli import _setup_llm_key

    _setup_llm_key(kb_dir)


async def _send(kb_dir: Path, session_id: str | None, message: str) -> dict[str, Any]:
    from agents import Runner
    from openkb.agent.chat_session import (
        ChatSession,
        load_session,
        resolve_session_id,
    )
    from openkb.agent.query import MAX_TURNS, build_chat_agent

    cfg = _load_kb_config(kb_dir)
    model = cfg.get("model", "deepseek/deepseek-chat")
    language = cfg.get("language", "zh")

    # 加载或新建 session
    if session_id:
        resolved = resolve_session_id(kb_dir, session_id)
        if not resolved:
            return {"ok": False, "error": f"session not found: {session_id}"}
        session = load_session(kb_dir, resolved)
    else:
        session = ChatSession.new(kb_dir, model=model, language=language)

    agent = build_chat_agent(kb_dir, model=model, language=language)
    new_input = session.history + [{"role": "user", "content": message}]

    # 非流式跑一轮（避免 TTY/stream UI 复杂度），最多 MAX_TURNS 个 agent step
    result = await Runner.run(agent, new_input, max_turns=MAX_TURNS)

    # 提取最终回答（agent SDK 的 final_output 或最后一条 assistant message）
    answer = ""
    if hasattr(result, "final_output") and result.final_output is not None:
        answer = str(result.final_output)
    if not answer:
        # 兜底：从 to_input_list 里取最后一条 assistant 文本
        try:
            full = result.to_input_list()
            for item in reversed(full):
                if item.get("role") == "assistant":
                    content = item.get("content", "")
                    if isinstance(content, list):
                        for c in content:
                            if isinstance(c, dict) and c.get("type") in ("text", "output_text"):
                                answer = c.get("text", "") or answer
                                break
                    elif isinstance(content, str):
                        answer = content
                    if answer:
                        break
        except Exception:
            pass

    # 保存新历史（record_turn 签名: user_message, assistant_text, new_history）
    new_history = result.to_input_list() if hasattr(result, "to_input_list") else new_input
    session.record_turn(message, answer, new_history)
    session.save()

    return {
        "ok": True,
        "session_id": session.id,
        "answer": answer or "(无回答)",
        "title": session.title or "",
    }


async def _stream(kb_dir: Path, session_id: str | None, message: str) -> None:
    """流式版本：每个 LLM token / 工具事件都按 NDJSON 一行写到 stdout。
    Go 后端读这个 stdout 转 SSE 推前端。

    输出事件类型：
      {"event":"start","session_id":"..."}
      {"event":"delta","text":"..."}            # 模型 token 增量
      {"event":"tool","name":"...","args":"..."} # agent 工具调用
      {"event":"done","session_id":"...","title":"...","answer":"..."}
      {"event":"error","error":"..."}
    """
    import sys

    def emit(obj: dict[str, Any]) -> None:
        sys.stdout.write(json.dumps(obj, ensure_ascii=False, default=str) + "\n")
        sys.stdout.flush()

    try:
        from agents import RawResponsesStreamEvent, RunItemStreamEvent, Runner
        from openai.types.responses import ResponseTextDeltaEvent
        from openkb.agent.chat_session import (
            ChatSession,
            load_session,
            resolve_session_id,
        )
        from openkb.agent.query import MAX_TURNS, build_chat_agent

        cfg = _load_kb_config(kb_dir)
        model = cfg.get("model", "deepseek/deepseek-chat")
        language = cfg.get("language", "zh")

        if session_id:
            resolved = resolve_session_id(kb_dir, session_id)
            if not resolved:
                emit({"event": "error", "error": f"session not found: {session_id}"})
                return
            session = load_session(kb_dir, resolved)
        else:
            session = ChatSession.new(kb_dir, model=model, language=language)

        emit({"event": "start", "session_id": session.id})

        agent = build_chat_agent(kb_dir, model=model, language=language)
        new_input = session.history + [{"role": "user", "content": message}]

        result = Runner.run_streamed(agent, new_input, max_turns=MAX_TURNS)
        collected: list[str] = []

        async for event in result.stream_events():
            if isinstance(event, RawResponsesStreamEvent):
                if isinstance(event.data, ResponseTextDeltaEvent):
                    delta = event.data.delta
                    if delta:
                        collected.append(delta)
                        emit({"event": "delta", "text": delta})
            elif isinstance(event, RunItemStreamEvent):
                item = event.item
                if item.type == "tool_call_item":
                    raw = item.raw_item
                    name = getattr(raw, "name", "?")
                    args = getattr(raw, "arguments", "") or ""
                    if not isinstance(args, str):
                        try:
                            args = json.dumps(args, ensure_ascii=False)
                        except Exception:
                            args = str(args)
                    if len(args) > 200:
                        args = args[:200] + "..."
                    emit({"event": "tool", "name": str(name), "args": args})

        answer = "".join(collected).strip()
        # final_output 优先：streaming SDK 的 RawResponsesStreamEvent 在多步 agent
        # （tool call → tool result → 再次 LLM）下不一定覆盖到所有段，导致 collected 拼出来的答案"看着像被截断"。
        # final_output 是 SDK 跑完整轮后的权威最终回答，长度更长就用它。
        try:
            final = getattr(result, "final_output", None)
            if final is not None:
                final_str = str(final).strip()
                if len(final_str) > len(answer):
                    answer = final_str
        except Exception:
            pass
        if not answer:
            answer = "(无回答)"

        # 持久化：与非流式 _send 同样调用 record_turn
        new_history = result.to_input_list() if hasattr(result, "to_input_list") else new_input
        session.record_turn(message, answer, new_history)
        session.save()

        emit({
            "event": "done",
            "session_id": session.id,
            "title": session.title or "",
            "answer": answer,
        })
    except Exception as exc:
        emit({"event": "error", "error": f"{type(exc).__name__}: {exc}", "trace": traceback.format_exc()})


def _list(kb_dir: Path) -> dict[str, Any]:
    from openkb.agent.chat_session import list_sessions

    return {"ok": True, "sessions": list_sessions(kb_dir)}


def _delete(kb_dir: Path, query: str) -> dict[str, Any]:
    from openkb.agent.chat_session import delete_session, resolve_session_id

    resolved = resolve_session_id(kb_dir, query)
    if not resolved:
        return {"ok": False, "error": f"no matching session: {query}"}
    ok = delete_session(kb_dir, resolved)
    return {"ok": ok, "session_id": resolved}


def _follow_ups(kb_dir: Path, user_q: str, answer: str, lang: str) -> dict[str, Any]:
    """让 LLM 生成 3 条「跟进问题」。

    走 LiteLLM 的 acompletion（OpenKB 已经依赖），由 LiteLLM 处理各家厂商协议
    （DeepSeek/OpenAI/Anthropic/Gemini/Azure/Bedrock/...），我们只关心 prompt 和 JSON 抽取。
    认证 / base url / model prefix 都靠 _setup_env 已写好的环境变量自动路由。
    """
    import re

    if not user_q or not answer:
        return {"ok": False, "error": "empty input"}

    user_q = user_q[:500]
    answer = answer[:1500]

    cfg = _load_kb_config(kb_dir)
    model = cfg.get("model", "deepseek/deepseek-chat")

    if lang == "en":
        system_prompt = (
            "You write follow-up questions a user is likely to ask next, "
            "after reading an assistant's answer.\n\n"
            "Rules:\n"
            "- Output strictly a JSON array of 3 strings, no markdown, no explanation.\n"
            "- Each question is short (<= 18 words), specific, and naturally extends the conversation.\n"
            "- Do NOT repeat the user's previous question.\n"
            "- Mix angles: deeper-dive, related-but-different, practical-application."
        )
    else:
        system_prompt = (
            "你是对话续写助手。读完用户问题和助手回答后，写出用户最可能继续问的 3 个跟进问题。\n\n"
            "要求：\n"
            "- 严格输出一个 JSON 数组，包含 3 个字符串，不要 markdown，不要解释。\n"
            "- 每条 ≤ 22 字，具体、自然延续对话。\n"
            "- 不要重复用户已问过的问题。\n"
            "- 尽量混合角度：深入追问、相关但不同的话题、实际应用。"
        )

    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": f"User question:\n{user_q}\n\nAssistant answer:\n{answer}"},
    ]

    # 优先用 litellm（OpenKB 已经依赖；它统一各厂商接口、读 _setup_env 设的环境变量自动路由）
    content = ""
    try:
        import litellm  # type: ignore

        resp = litellm.completion(
            model=model,
            messages=messages,
            temperature=0.7,
            max_tokens=256,
            timeout=12,
        )
        # litellm 返回 OpenAI 兼容的 ModelResponse
        content = resp["choices"][0]["message"]["content"] or ""
    except Exception as exc:
        return {"ok": False, "error": f"litellm: {type(exc).__name__}: {exc}"}

    # 解析 JSON 数组（兼容裸 JSON / ```json``` 包裹 / 前后带解释）
    suggestions: list[str] = []
    content = content.strip()
    if content:
        # 1) 直接尝试
        try:
            parsed = json.loads(content)
            if isinstance(parsed, list):
                suggestions = [str(s).strip() for s in parsed if str(s).strip()]
        except Exception:
            # 2) 找出第一个 [ "..." , ... ] 数组
            m = re.search(r'\[\s*"[^"]*"(?:\s*,\s*"[^"]*")*\s*\]', content, re.DOTALL)
            if m:
                try:
                    parsed = json.loads(m.group(0))
                    if isinstance(parsed, list):
                        suggestions = [str(s).strip() for s in parsed if str(s).strip()]
                except Exception:
                    pass

    # 去重 + 截断 3 条
    seen: set[str] = set()
    out: list[str] = []
    for s in suggestions:
        if s not in seen:
            seen.add(s)
            out.append(s)
        if len(out) >= 3:
            break

    return {"ok": True, "follow_ups": out, "model": model}


def _load(kb_dir: Path, session_id: str) -> dict[str, Any]:
    """返回 session 完整对话（前端渲染消息列表）。

    优先用 OpenKB 存好的 user_turns + assistant_texts（人话，已配对），
    比从 agent SDK 的 history 抽 content 字段稳得多——后者含 tool_call/tool_output 等噪音。
    """
    from openkb.agent.chat_session import load_session, resolve_session_id

    resolved = resolve_session_id(kb_dir, session_id)
    if not resolved:
        return {"ok": False, "error": f"session not found: {session_id}"}
    session = load_session(kb_dir, resolved)

    msgs: list[dict[str, str]] = []
    user_turns = list(getattr(session, "user_turns", []) or [])
    assistant_texts = list(getattr(session, "assistant_texts", []) or [])
    # 严格按时间顺序穿插：user_turns[i] → assistant_texts[i]
    for i in range(max(len(user_turns), len(assistant_texts))):
        if i < len(user_turns):
            msgs.append({"role": "user", "content": user_turns[i]})
        if i < len(assistant_texts):
            msgs.append({"role": "assistant", "content": assistant_texts[i]})

    return {
        "ok": True,
        "session_id": session.id,
        "title": session.title or "",
        "messages": msgs,
    }


def main() -> None:
    try:
        req = json.loads(sys.stdin.read())
    except Exception as exc:
        print(json.dumps({"ok": False, "error": f"bad input: {exc}"}, ensure_ascii=False))
        return

    action = req.get("action")
    kb_dir = Path(req.get("kb_dir", "")).resolve()
    if not kb_dir.exists():
        print(json.dumps({"ok": False, "error": f"kb_dir not found: {kb_dir}"}, ensure_ascii=False))
        return

    _setup_env(kb_dir)

    try:
        if action == "send":
            result = asyncio.run(_send(kb_dir, req.get("session_id"), req.get("message", "")))
        elif action == "stream":
            # stream 模式自己负责按行 emit，不走最终 print
            asyncio.run(_stream(kb_dir, req.get("session_id"), req.get("message", "")))
            return
        elif action == "list":
            result = _list(kb_dir)
        elif action == "delete":
            result = _delete(kb_dir, req.get("session_id", ""))
        elif action == "load":
            result = _load(kb_dir, req.get("session_id", ""))
        elif action == "follow_ups":
            result = _follow_ups(
                kb_dir,
                req.get("user_q", ""),
                req.get("answer", ""),
                req.get("lang", "zh-CN"),
            )
        else:
            result = {"ok": False, "error": f"unknown action: {action}"}
    except Exception as exc:
        result = {"ok": False, "error": f"{type(exc).__name__}: {exc}", "trace": traceback.format_exc()}

    print(json.dumps(result, ensure_ascii=False, default=str))


if __name__ == "__main__":
    main()
