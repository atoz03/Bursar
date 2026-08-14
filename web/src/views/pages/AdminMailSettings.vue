<template>
  <el-card class="card">
    <template #header>
      <div class="head">
        <el-icon><Message /></el-icon>
        <span>邮箱发送配置</span>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
    <el-alert
      :title="smtpPasswordSet ? 'SMTP 密码已保存。若需更新密码，请在下方重新输入后点击保存。' : 'SMTP 密码尚未保存，请填写后点击保存。'"
      :type="smtpPasswordSet ? 'info' : 'warning'"
      :closable="false"
      show-icon
      class="mb"
    />

    <el-form label-position="top">
      <el-row :gutter="12">
        <el-col :span="12"><el-form-item label="SMTP 主机 *" required><el-input v-model="form.smtp_host" placeholder="smtp.example.org" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="SMTP 端口 *" required><el-input-number v-model="form.smtp_port" :min="1" :max="65535" style="width: 100%" /></el-form-item></el-col>
      </el-row>
      <el-row :gutter="12">
        <el-col :span="12"><el-form-item label="SMTP 用户名 *" required><el-input v-model="form.smtp_user" placeholder="mailer@example.org" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="SMTP 密码"><el-input v-model="smtpPass" type="password" show-password placeholder="留空表示不修改" /></el-form-item></el-col>
      </el-row>
      <el-row :gutter="12">
        <el-col :span="12"><el-form-item label="发件邮箱 *" required><el-input v-model="form.from_email" placeholder="建议与 SMTP 用户名一致" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="发件人名称 *" required><el-input v-model="form.from_name" /></el-form-item></el-col>
      </el-row>
    </el-form>

    <el-button type="primary" :loading="saving" @click="save">保存设置</el-button>
    <el-divider />
    <el-form inline>
      <el-form-item label="测试用户名">
        <el-select
          v-model="testUsername"
          filterable
          clearable
          remote
          :remote-method="onUserSearch"
          :loading="usersLoading"
          placeholder="输入用户名进行匹配"
          style="width: 320px"
        >
          <el-option
            v-for="u in visibleUserOptions"
            :key="u.username"
            :label="`${u.username}（${u.real_name || '未填写姓名'}）`"
            :value="u.username"
          />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button :loading="testing" @click="sendTest">发送测试邮件</el-button>
      </el-form-item>
    </el-form>
    <el-descriptions v-if="selectedTestUser" :column="2" border style="margin-top: 8px">
      <el-descriptions-item label="平台账号">{{ selectedTestUser.username }}</el-descriptions-item>
      <el-descriptions-item label="真实姓名">{{ selectedTestUser.real_name || "-" }}</el-descriptions-item>
      <el-descriptions-item label="邮箱">{{ selectedTestUser.email || "-" }}</el-descriptions-item>
      <el-descriptions-item label="学号">{{ selectedTestUser.student_id || "-" }}</el-descriptions-item>
      <el-descriptions-item label="账号类型">{{ roleText(selectedTestUser.role) }}</el-descriptions-item>
      <el-descriptions-item label="当前状态">{{ selectedTestUser.status || "-" }}</el-descriptions-item>
    </el-descriptions>

    <el-divider />
    <el-card>
      <template #header>
        <div class="section-inline-title">
          <el-icon><Bell /></el-icon>
          <span>通知邮件发送</span>
        </div>
      </template>
      <el-form label-position="top">
        <el-form-item>
          <el-checkbox v-model="mailAllUsers">发送给全部普通用户</el-checkbox>
        </el-form-item>
        <el-form-item label="指定用户">
          <el-select
            v-model="mailUsernames"
            multiple
            filterable
            clearable
            remote
            :remote-method="onUserSearch"
            :loading="usersLoading"
            placeholder="输入平台账号进行匹配"
            style="width: 100%"
            :disabled="mailAllUsers"
          >
            <el-option
              v-for="u in visibleUserOptions"
              :key="u.username"
              :label="`${u.username}（${u.real_name || '未填写姓名'}，${u.email || '无邮箱'}）`"
              :value="u.username"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="邮件主题">
          <el-input v-model="mailSubject" placeholder="例如：平台维护通知" />
        </el-form-item>
        <el-form-item label="邮件正文">
          <el-input
            v-model="mailBody"
            type="textarea"
            :rows="7"
            placeholder="支持变量：{{username}}、{{real_name}}"
          />
        </el-form-item>
        <el-form-item>
          <el-button :loading="sendingBulk" type="primary" @click="sendBulkMail">发送通知邮件</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </el-card>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { ApiClient, type AdminUserDetail } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { Bell, Message } from "@element-plus/icons-vue";

const saving = ref(false);
const testing = ref(false);
const error = ref("");
const success = ref("");
const smtpPass = ref("");
const testUsername = ref("");
const usersLoading = ref(false);
const allUsers = ref<AdminUserDetail[]>([]);
const userKeyword = ref("");
const smtpPasswordSet = ref(false);
const sendingBulk = ref(false);
const mailAllUsers = ref(false);
const mailUsernames = ref<string[]>([]);
const mailSubject = ref(`${authState.platformName} 平台通知`);
const mailBody = ref(`你好 {{real_name}}（{{username}}）：\n\n这里是平台通知内容。\n\n${authState.platformName} 团队`);

const form = reactive({
  smtp_host: "",
  smtp_port: 587,
  smtp_user: "",
  from_email: "",
  from_name: authState.platformName,
});

const visibleUserOptions = computed(() => {
  const k = userKeyword.value.trim().toLowerCase();
  const rows = (allUsers.value ?? []).filter((x) => x.role === "user" && String(x.email || "").trim() !== "");
  if (!k) return rows.slice(0, 100);
  return rows
    .filter((x) =>
      (x.username ?? "").toLowerCase().includes(k) ||
      (x.real_name ?? "").toLowerCase().includes(k) ||
      (x.email ?? "").toLowerCase().includes(k) ||
      (x.student_id ?? "").toLowerCase().includes(k),
    )
    .slice(0, 100);
});

const selectedTestUser = computed(() => {
  const u = testUsername.value.trim();
  if (!u) return null;
  return allUsers.value.find((x) => x.username === u) ?? null;
});

function roleText(role: string): string {
  if (role === "admin") return "管理员";
  if (role === "power_user") return "高级用户";
  return "普通用户";
}

async function load() {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const mail = await client.adminGetMailSettings();
	form.smtp_host = mail.smtp_host || "";
	form.smtp_port = mail.smtp_port || 587;
  form.smtp_user = mail.smtp_user ?? "";
  smtpPasswordSet.value = !!mail.smtp_password_set;
  form.from_email = mail.from_email || form.smtp_user;
	form.from_name = mail.from_name || authState.platformName;
  try {
    usersLoading.value = true;
    const users = await client.adminUsersDetails(2000);
    allUsers.value = users.users ?? [];
  } finally {
    usersLoading.value = false;
  }
}

async function loadUsers() {
  usersLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminUsersDetails(2000);
    allUsers.value = r.users ?? [];
  } finally {
    usersLoading.value = false;
  }
}

function onUserSearch(query: string) {
  userKeyword.value = query || "";
  if (allUsers.value.length === 0 && !usersLoading.value) {
    loadUsers().catch(() => {});
  }
}

async function save() {
  error.value = "";
  success.value = "";
  saving.value = true;
  try {
    if (!String(form.smtp_host || "").trim()) {
      error.value = "请填写 SMTP 主机";
      return;
    }
    if (!String(form.smtp_user || "").trim()) {
      error.value = "请填写 SMTP 用户名";
      return;
    }
    if (!String(form.from_email || "").trim()) {
      error.value = "请填写发件邮箱";
      return;
    }
    if (smtpPass.value && !String(smtpPass.value).trim()) {
      error.value = "SMTP 密码不能为空";
      return;
    }
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminSetMailSettings({
      ...form,
      smtp_pass: smtpPass.value,
      update_pass: !!smtpPass.value,
    });
    success.value = "保存成功";
    smtpPass.value = "";
    smtpPasswordSet.value = true;
    await load();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    saving.value = false;
  }
}

async function sendTest() {
  error.value = "";
  success.value = "";
  if (!testUsername.value.trim()) {
    error.value = "请填写测试用户名";
    return;
  }
  if (!selectedTestUser.value) {
    error.value = "请选择一个已开通邮箱的普通平台用户";
    return;
  }
  const expectedEmail = String(selectedTestUser.value.email || "").trim();
  if (!expectedEmail) {
    error.value = "该平台用户未配置邮箱，无法发送测试邮件";
    return;
  }
  testing.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminMailTest(testUsername.value.trim());
    success.value = `测试邮件已发送到 ${r.email}（平台用户登记邮箱：${expectedEmail}）`;
  } catch (e: any) {
    const msg = String(e?.message ?? String(e)).trim();
    if (e?.status === 404 && msg === "请求的资源不存在") {
      const base = (settingsState.baseUrl || window.location.origin || "").trim() || window.location.origin;
      error.value = `测试邮件接口不可用：当前控制器地址为 ${base}，请确认该实例已更新并重启。`;
      return;
    }
    error.value = `发送失败（目标邮箱：${expectedEmail}）：${msg || "未知错误"}`;
  } finally {
    testing.value = false;
  }
}

async function sendBulkMail() {
  error.value = "";
  success.value = "";
  if (!mailAllUsers.value && mailUsernames.value.length === 0) {
    error.value = "请先选择目标用户，或勾选“发送给全部普通用户”";
    return;
  }
  if (!String(mailSubject.value || "").trim()) {
    error.value = "请填写邮件主题";
    return;
  }
  if (!String(mailBody.value || "").trim()) {
    error.value = "请填写邮件正文";
    return;
  }
  sendingBulk.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminMailSend({
      all_users: mailAllUsers.value,
      usernames: mailUsernames.value,
      subject: mailSubject.value.trim(),
      body: mailBody.value.trim(),
    });
    success.value = `发送完成：总计 ${r.total}，成功 ${r.success}，失败 ${r.failed}`;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    sendingBulk.value = false;
  }
}

load().catch((e: any) => {
  error.value = e?.message ?? String(e);
});
</script>

<style scoped>
.card { max-width: 980px; }
.head { display: flex; align-items: center; gap: 8px; font-weight: 700; }
.section-inline-title { display: inline-flex; align-items: center; gap: 8px; font-weight: 700; }
.mb { margin-bottom: 12px; }
</style>
