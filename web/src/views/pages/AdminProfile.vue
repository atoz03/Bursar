<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><UserFilled /></el-icon></span>
          <div>
          <div class="title">管理员个人信息</div>
          <div class="sub">仅修改当前登录管理员/高级用户自己的资料</div>
          </div>
        </div>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />

    <el-form label-width="120px" style="max-width: 680px">
      <el-form-item label="登录账号">
        <el-input v-model="form.username" disabled />
      </el-form-item>
      <el-form-item label="真实姓名">
        <el-input v-model="form.real_name" placeholder="请输入真实姓名" />
      </el-form-item>
      <el-form-item label="邮箱">
        <el-input v-model="form.email" placeholder="可选" />
      </el-form-item>
      <el-form-item label="电话">
        <el-input v-model="form.phone" placeholder="可选" />
      </el-form-item>
      <el-form-item>
        <el-button :loading="loading" type="primary" @click="save">保存</el-button>
      </el-form-item>
    </el-form>

    <TwoFactorSettingsCard />
  </el-card>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { UserFilled } from "@element-plus/icons-vue";
import TwoFactorSettingsCard from "../../components/TwoFactorSettingsCard.vue";

const loading = ref(false);
const error = ref("");
const success = ref("");
const form = reactive({
  username: "",
  real_name: "",
  email: "",
  phone: "",
});

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const r = await client().adminMyProfile();
    form.username = r.profile.username || "";
    form.real_name = r.profile.real_name || "";
    form.email = r.profile.email || "";
    form.phone = r.profile.phone || "";
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function save() {
  loading.value = true;
  error.value = "";
  success.value = "";
  try {
    await client().adminSetMyProfile({
      real_name: form.real_name,
      email: form.email,
      phone: form.phone,
    });
    success.value = "保存成功";
    await load();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

load();
</script>

<style scoped>
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.section-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.section-icon {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #dbeafe;
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
}
.title {
  font-weight: 700;
}
.sub {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
}
.mb {
  margin-bottom: 12px;
}
</style>
