import { createApp } from "vue";
import ElementPlus from "element-plus";
import "element-plus/dist/index.css";
import "./styles/global.css";
import App from "./App.vue";
import { router } from "./router";
import { setupMathSupport } from "./lib/math";

setupMathSupport();
createApp(App).use(router).use(ElementPlus).mount("#app");
