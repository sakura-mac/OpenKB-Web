import { createApp } from 'vue'
import App from './App.vue'
import { i18n, detectLocaleFromServer } from './i18n'
import './style.css'

// 应用挂载后立即去问后端"按 IP 你认为我是中文用户还是英文用户"。
// 这是非阻塞的——先按浏览器/storage 渲染，再可能切换一次。
detectLocaleFromServer()

createApp(App).use(i18n).mount('#app')
