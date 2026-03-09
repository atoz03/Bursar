<template>
  <el-card class="two-factor-card">
    <template #header>
      <div class="row">
        <div>
          <h3 class="title">双重验证（2FA）</h3>
          <p class="sub">支持 Microsoft Authenticator、数盾等标准 TOTP 验证器</p>
        </div>
        <el-button :loading="loading" @click="loadStatus">刷新</el-button>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
    <el-alert
      v-if="status && !status.enabled"
      title="当前未开启 2FA。建议尽快开启，登录时可额外校验动态验证码。"
      type="warning"
      show-icon
      :closable="false"
      class="mb"
    />
    <el-alert
      v-else-if="status?.enabled"
      title="当前账号已启用 2FA。登录时需要同时校验密码、登录验证码和 6 位动态码。"
      type="success"
      show-icon
      :closable="false"
      class="mb"
    />

    <el-descriptions v-if="status" :column="2" border class="mb">
      <el-descriptions-item label="当前状态">
        <el-tag :type="status.enabled ? 'success' : 'info'">{{ status.enabled ? "已开启" : "未开启" }}</el-tag>
      </el-descriptions-item>
      <el-descriptions-item label="账号标识">{{ status.account_name }}</el-descriptions-item>
      <el-descriptions-item label="验证器发行方">{{ status.issuer }}</el-descriptions-item>
      <el-descriptions-item label="待确认配置">{{ status.pending_setup ? "有" : "无" }}</el-descriptions-item>
    </el-descriptions>

    <template v-if="!status?.enabled">
      <el-space wrap class="mb">
        <el-button type="primary" :loading="setupLoading" @click="beginSetup">{{ status?.pending_setup ? "重新生成密钥" : "开始配置 2FA" }}</el-button>
      </el-space>

      <el-alert
        v-if="status?.pending_setup && !setupData"
        title="检测到有未完成的 2FA 配置。如果你看不到之前的密钥，请重新生成新的密钥后再完成验证。"
        type="info"
        show-icon
        :closable="false"
        class="mb"
      />

      <div v-if="setupData" class="setup-box">
        <el-alert
          title="请在验证器中手动添加 TOTP 账户，再输入当前 6 位动态码完成开启。"
          type="info"
          show-icon
          :closable="false"
          class="mb"
        />
        <el-descriptions :column="1" border class="mb">
          <el-descriptions-item label="账户名">{{ setupData.account_name }}</el-descriptions-item>
          <el-descriptions-item label="手动输入密钥">
            <div class="secret-wrap">
              <code>{{ setupData.secret }}</code>
              <el-button text type="primary" @click="copyText(setupData.secret)">复制</el-button>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="OTPAUTH 链接">
            <div class="secret-wrap">
              <code class="uri">{{ setupData.otpauth_url }}</code>
              <el-button text type="primary" @click="copyText(setupData.otpauth_url)">复制</el-button>
            </div>
          </el-descriptions-item>
        </el-descriptions>

        <el-form label-position="top" style="max-width: 420px">
          <el-form-item label="当前 6 位动态码">
            <el-input v-model="enableCode" maxlength="6" placeholder="请输入验证器当前显示的 6 位动态码" />
          </el-form-item>
        </el-form>
        <el-button type="primary" :loading="enableLoading" @click="confirmEnable">确认开启 2FA</el-button>
      </div>
    </template>

    <template v-else>
      <el-form label-position="top" style="max-width: 420px">
        <el-form-item label="当前密码">
          <el-input v-model="disablePassword" type="password" show-password placeholder="关闭 2FA 时需要再次验证密码" />
        </el-form-item>
        <el-form-item label="当前 6 位动态码">
          <el-input v-model="disableCode" maxlength="6" placeholder="请输入验证器当前显示的 6 位动态码" />
        </el-form-item>
      </el-form>
      <el-button type="danger" :loading="disableLoading" @click="disableTwoFactor">关闭 2FA</el-button>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { ApiClient, type TwoFactorSetup, type TwoFactorState } from "../lib/api";
import { authState, refreshAuth } from "../lib/authStore";
import { settingsState } from "../lib/settingsStore";

const loading = ref(false);
const setupLoading = ref(false);
const enableLoading = ref(false);
const disableLoading = ref(false);
const error = ref("");
const success = ref("");
const status = ref<TwoFactorState | null>(null);
const setupData = ref<TwoFactorSetup | null>(null);
const enableCode = ref("");
const disablePassword = ref("");
const disableCode = ref("");

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

async function loadStatus() {
  loading.value = true;
  error.value = "";
  try {
    status.value = await client().auth2faStatus();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function beginSetup() {
  setupLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    setupData.value = await client().auth2faSetup();
    status.value = await client().auth2faStatus();
    enableCode.value = "";
    success.value = "新的 2FA 密钥已生成，请在验证器中添加后完成确认。";
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    setupLoading.value = false;
  }
}

async function confirmEnable() {
  enableLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    await client().auth2faEnable(enableCode.value.trim());
    setupData.value = null;
    enableCode.value = "";
    await Promise.all([loadStatus(), refreshAuth()]);
    success.value = "2FA 已开启。后续登录需要输入动态验证码。";
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    enableLoading.value = false;
  }
}

async function disableTwoFactor() {
  disableLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    await client().auth2faDisable(disablePassword.value, disableCode.value.trim());
    disablePassword.value = "";
    disableCode.value = "";
    await Promise.all([loadStatus(), refreshAuth()]);
    success.value = "2FA 已关闭。";
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    disableLoading.value = false;
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(String(text || ""));
    ElMessage.success("已复制");
  } catch {
    ElMessage.error("复制失败，请手动复制");
  }
}

onMounted(() => {
  void loadStatus();
});
</script>

<style scoped>
.two-factor-card {
  margin-top: 16px;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.title {
  margin: 0;
  font-size: 18px;
  color: #0f172a;
}
.sub {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
}
.mb {
  margin-bottom: 12px;
}
.setup-box {
  padding: 14px;
  border: 1px solid #dbeafe;
  border-radius: 14px;
  background: linear-gradient(180deg, #f8fbff 0%, #eef6ff 100%);
}
.secret-wrap {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.secret-wrap code {
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
}
.uri {
  max-width: 100%;
}

@media (max-width: 900px) {
  .row,
  .secret-wrap {
    flex-wrap: wrap;
  }
}
</style>
