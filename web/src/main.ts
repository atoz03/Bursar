import { createApp } from "vue";
import ElementPlus from "element-plus";
import zhCn from "element-plus/es/locale/lang/zh-cn";
import "element-plus/dist/index.css";
import "./styles/global.css";
import App from "./App.vue";
import { router } from "./router";
import { setupMathSupport } from "./lib/math";

setupMathSupport();
createApp(App).use(router).use(ElementPlus, { locale: zhCn }).mount("#app");
