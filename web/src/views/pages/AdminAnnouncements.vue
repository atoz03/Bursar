<template>
  <el-card>
    <template #header>
      <div class="row">
        <div><div class="title">公告管理</div><div class="sub">发布后用户首页可见</div></div>
        <el-button type="primary" :loading="loading" @click="reload">刷新</el-button>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />

    <el-form label-position="top">
      <el-form-item label="标题"><el-input v-model="title" /></el-form-item>
      <el-form-item label="内容"><el-input v-model="content" type="textarea" :rows="4" /></el-form-item>
      <el-form-item><el-checkbox v-model="pinned">置顶</el-checkbox></el-form-item>
      <el-form-item><el-button type="primary" :loading="publishing" @click="publish">发布公告</el-button></el-form-item>
    </el-form>

    <el-table :data="rows" stripe>
      <el-table-column prop="announcement_id" label="ID" width="80" />
      <el-table-column prop="title" label="标题" width="220" />
      <el-table-column prop="pinned" label="置顶" width="80">
        <template #default="{row}">{{ row.pinned ? '是' : '否' }}</template>
      </el-table-column>
      <el-table-column prop="created_by" label="发布人" width="120" />
      <el-table-column prop="created_at" label="发布时间" width="180" />
      <el-table-column prop="content" label="内容" min-width="260" />
      <el-table-column label="操作" width="120">
        <template #default="{row}">
          <el-button type="danger" size="small" @click="remove(row.announcement_id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { ApiClient, type Announcement } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";

const loading = ref(false);
const publishing = ref(false);
const error = ref("");
const rows = ref<Announcement[]>([]);
const title = ref("");
const content = ref("");
const pinned = ref(false);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.announcements(50);
    rows.value = r.announcements ?? [];
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  } finally {
    loading.value = false;
  }
}

async function publish() {
  publishing.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminCreateAnnouncement({ title: title.value.trim(), content: content.value.trim(), pinned: pinned.value });
    title.value = "";
    content.value = "";
    pinned.value = false;
    await reload();
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  } finally {
    publishing.value = false;
  }
}

async function remove(id: number) {
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminDeleteAnnouncement(id);
    await reload();
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  }
}

reload();
</script>

<style scoped>
.row { display: flex; justify-content: space-between; align-items: center; }
.title { font-weight: 700; }
.sub { color:#64748b; font-size:12px; }
.mb { margin-bottom: 12px; }
</style>
