<template>
  <el-card>
    <template #header>
      <div class="row">
        <div>
          <div class="title">个人信息</div>
          <div class="sub">可直接修改基础信息；用户名/邮箱/学号变更需管理员审核</div>
        </div>
        <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
      </div>
    </template>

    <el-alert
      title="提示：修改“用户名、邮箱、学号”时，必须填写备注说明，提交后进入管理员审核。"
      type="warning"
      show-icon
      class="mb"
    />
    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />

    <el-form label-position="top">
      <el-row :gutter="12">
        <el-col :span="12"><el-form-item label="邮箱 *" required><el-input v-model="form.email" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="用户名 *" required><el-input v-model="form.username" /></el-form-item></el-col>
      </el-row>
      <el-row :gutter="12">
        <el-col :span="12"><el-form-item label="真实姓名 *" required><el-input v-model="form.real_name" /></el-form-item></el-col>
        <el-col :span="12"><el-form-item label="学号 *" required><el-input v-model="form.student_id" /></el-form-item></el-col>
      </el-row>
      <el-row :gutter="12">
        <el-col :span="12"><el-form-item label="导师 *" required><el-input v-model="form.advisor" /></el-form-item></el-col>
        <el-col :span="12">
          <el-form-item label="预计毕业年份 *" required>
            <el-input-number v-model="form.expected_graduation_year" :min="2000" :max="2200" style="width: 100%" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-form-item label="电话 *" required><el-input v-model="form.phone" /></el-form-item>
      <el-form-item label="变更备注（仅修改用户名/邮箱/学号时必填）">
        <el-input v-model="form.change_reason" type="textarea" :rows="3" placeholder="请说明变更原因，供管理员审核" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </el-form-item>
    </el-form>

    <el-divider />
    <div class="title">关键信息变更申请记录</div>
    <el-table :data="requests" stripe>
      <el-table-column prop="request_id" label="ID" width="90" />
      <el-table-column prop="status" label="状态" width="120" />
      <el-table-column prop="old_username" label="原用户名" width="130" />
      <el-table-column prop="new_username" label="新用户名" width="130" />
      <el-table-column prop="old_email" label="原邮箱" min-width="180" />
      <el-table-column prop="new_email" label="新邮箱" min-width="180" />
      <el-table-column prop="old_student_id" label="原学号" width="130" />
      <el-table-column prop="new_student_id" label="新学号" width="130" />
      <el-table-column prop="reason" label="备注" min-width="200" />
      <el-table-column prop="created_at" label="提交时间" width="180" />
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { ApiClient, type ProfileChangeRequest } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";

const loading = ref(false);
const saving = ref(false);
const error = ref("");
const success = ref("");
const requests = ref<ProfileChangeRequest[]>([]);

const form = reactive({
  email: "",
  username: "",
  real_name: "",
  student_id: "",
  advisor: "",
  expected_graduation_year: new Date().getFullYear() + 3,
  phone: "",
  change_reason: "",
});

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl);
    const [me, reqs] = await Promise.all([client.userMe(), client.userProfileChangeRequests(100)]);
    form.email = me.email ?? "";
    form.username = me.username ?? "";
    form.real_name = me.real_name ?? "";
    form.student_id = me.student_id ?? "";
    form.advisor = me.advisor ?? "";
    form.expected_graduation_year = me.expected_graduation_year ?? new Date().getFullYear() + 3;
    form.phone = me.phone ?? "";
    requests.value = reqs.requests ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl);
    const r = await client.userUpdateProfile({ ...form });
    success.value = r.message || "保存成功";
    form.change_reason = "";
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    saving.value = false;
  }
}

reload();
</script>

<style scoped>
.row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.title { font-weight: 700; }
.sub { margin-top: 4px; font-size: 12px; color: #64748b; }
.mb { margin-bottom: 12px; }
</style>
