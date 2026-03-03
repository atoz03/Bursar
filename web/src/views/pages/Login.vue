<template>
  <div class="login-shell">
    <div class="aurora aurora-a" />
    <div class="aurora aurora-b" />

    <div class="login-layout">
      <section class="intro">
        <div class="intro-badge">GPU Ops</div>
        <h1>欢迎登录平台</h1>
        <p>统一管理 GPU 资源、平台账号与计费状态。</p>
      </section>

      <section class="panel">
        <div class="brand">
          <el-icon :size="40"><Cpu /></el-icon>
          <div>
            <h2>账号登录</h2>
            <p>请输入平台账号与密码</p>
          </div>
        </div>

        <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />

        <el-form label-position="top" class="login-form">
          <el-form-item label="用户名">
            <el-input
              v-model="username"
              size="large"
              autocomplete="username"
              :prefix-icon="User"
              placeholder="请输入平台账号"
              @keyup.enter="doLogin"
            />
          </el-form-item>
          <el-form-item label="密码">
            <el-input
              v-model="password"
              size="large"
              type="password"
              show-password
              autocomplete="current-password"
              :prefix-icon="Key"
              placeholder="请输入密码"
              @keyup.enter="doLogin"
            />
          </el-form-item>
        </el-form>

        <el-button :loading="loading" type="primary" class="login-btn" size="large" @click="doLogin">立即登录</el-button>

        <div class="actions">
          <router-link to="/register">注册申请</router-link>
          <router-link to="/forgot-password">找回密码</router-link>
        </div>
      </section>
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
        await router.push("/user/balance");
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
.login-shell {
  position: relative;
  min-height: 100vh;
  padding: 28px;
  display: grid;
  place-items: center;
  overflow: hidden;
  background: linear-gradient(130deg, #073b4c 0%, #0f766e 45%, #d9f99d 100%);
}
.aurora {
  position: absolute;
  border-radius: 999px;
  filter: blur(40px);
  opacity: 0.45;
  pointer-events: none;
}
.aurora-a {
  width: 360px;
  height: 360px;
  left: -80px;
  top: -90px;
  background: #34d399;
  animation: flow 12s ease-in-out infinite;
}
.aurora-b {
  width: 420px;
  height: 420px;
  right: -110px;
  bottom: -130px;
  background: #f59e0b;
  animation: flow 14s ease-in-out infinite reverse;
}
.login-layout {
  position: relative;
  z-index: 1;
  width: min(1100px, 96vw);
  display: grid;
  grid-template-columns: 1.05fr 0.95fr;
  gap: 20px;
}
.intro {
  padding: 42px 34px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.16);
  border: 1px solid rgba(255, 255, 255, 0.28);
  color: #f8fafc;
  backdrop-filter: blur(6px);
}
.intro-badge {
  display: inline-block;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.8px;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.2);
}
.intro h1 {
  margin: 20px 0 8px;
  font-size: 44px;
  line-height: 1.14;
}
.intro p {
  margin: 0;
  font-size: 19px;
  line-height: 1.6;
  opacity: 0.95;
}
.panel {
  padding: 28px 24px 22px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(8px);
  box-shadow: 0 20px 50px rgba(2, 6, 23, 0.22);
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  color: #0f172a;
  margin-bottom: 12px;
}
.brand h2 {
  margin: 0;
  font-size: 28px;
}
.brand p {
  margin: 3px 0 0;
  color: #475569;
  font-size: 15px;
}
.mb {
  margin-bottom: 12px;
}
.login-form :deep(.el-form-item__label) {
  font-size: 15px;
  font-weight: 700;
}
.login-btn {
  width: 100%;
  margin-top: 8px;
  height: 48px;
  font-size: 17px;
  font-weight: 700;
}
.actions {
  margin-top: 14px;
  display: flex;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.actions a {
  color: #0f766e;
  text-decoration: none;
  font-weight: 700;
  font-size: 14px;
}
.actions a:hover {
  text-decoration: underline;
}
@keyframes flow {
  0%,
  100% {
    transform: translate(0, 0);
  }
  50% {
    transform: translate(18px, -12px);
  }
}
@media (max-width: 920px) {
  .login-layout {
    grid-template-columns: 1fr;
  }
  .intro {
    padding: 26px 20px;
  }
  .intro h1 {
    font-size: 34px;
  }
  .intro p {
    font-size: 17px;
  }
}
</style>
