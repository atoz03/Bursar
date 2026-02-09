<template>
  <div class="water-shell">
    <div class="water-bg">
      <span class="wave wave-1"></span>
      <span class="wave wave-2"></span>
      <span class="wave wave-3"></span>
    </div>

    <div class="login-panel">
      <div class="brand">
        <el-icon :size="34"><Cpu /></el-icon>
        <div>
          <h1>GPU Ops</h1>
          <p>GPU 运维与计费平台</p>
        </div>
      </div>

      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />

      <el-form label-position="top">
        <el-form-item label="用户名">
          <el-input v-model="username" autocomplete="username" :prefix-icon="User" @keyup.enter="doLogin" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="password" type="password" show-password autocomplete="current-password" :prefix-icon="Key" @keyup.enter="doLogin" />
        </el-form-item>
      </el-form>

      <el-button :loading="loading" type="primary" class="login-btn" @click="doLogin">登录</el-button>

      <div class="actions">
        <router-link to="/register">用户注册</router-link>
        <router-link to="/forgot-password">找回密码</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { login, authState } from "../../lib/authStore";
import { Cpu, Key, User } from "@element-plus/icons-vue";

const router = useRouter();
const loading = ref(false);
const error = ref("");
const username = ref("");
const password = ref("");

async function doLogin() {
  loading.value = true;
  error.value = "";
  try {
    await login(username.value.trim(), password.value);
    if (authState.role === "admin") {
      await router.push("/admin/board");
    } else if (authState.role === "power_user") {
      if (authState.canViewBoard) {
        await router.push("/admin/board");
      } else if (authState.canViewNodes) {
        await router.push("/admin/nodes");
      } else if (authState.canReviewRequests) {
        await router.push("/admin/requests");
      } else {
        error.value = "当前高级用户未授予可访问权限，请联系管理员";
      }
    } else {
      await router.push("/user/balance");
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.water-shell {
  position: relative;
  overflow: hidden;
  min-height: 100vh;
  display: grid;
  place-items: center;
  background: linear-gradient(145deg, #0b3948 0%, #087e8b 42%, #bfdbf7 100%);
}
.water-bg {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.wave {
  position: absolute;
  left: -30%;
  width: 160%;
  height: 55%;
  border-radius: 44%;
  background: rgba(255, 255, 255, 0.15);
  animation: drift 16s linear infinite;
}
.wave-1 { bottom: -28%; }
.wave-2 { bottom: -34%; opacity: 0.22; animation-duration: 20s; animation-direction: reverse; }
.wave-3 { bottom: -40%; opacity: 0.12; animation-duration: 26s; }
.login-panel {
  position: relative;
  z-index: 1;
  width: min(92vw, 460px);
  padding: 26px 24px 18px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.78);
  backdrop-filter: blur(10px);
  box-shadow: 0 20px 50px rgba(2, 6, 23, 0.24);
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  color: #0f172a;
  margin-bottom: 12px;
}
.brand h1 {
  margin: 0;
  font-size: 24px;
  letter-spacing: 0.4px;
}
.brand p {
  margin: 2px 0 0;
  color: #334155;
  font-size: 13px;
}
.mb {
  margin-bottom: 12px;
}
.login-btn {
  width: 100%;
  margin-top: 4px;
}
.actions {
  margin-top: 14px;
  display: flex;
  justify-content: space-between;
}
.actions a {
  color: #0f766e;
  text-decoration: none;
  font-weight: 600;
}
@keyframes drift {
  0% { transform: translateX(-8%) rotate(0deg); }
  100% { transform: translateX(8%) rotate(360deg); }
}
@media (max-width: 700px) {
  .login-panel {
    padding: 20px 14px 14px;
  }
}
</style>
