<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><Bell /></el-icon></span>
          <div><div class="title">公告管理</div><div class="sub">发布后用户“公告与用户准则”页面可见</div></div>
        </div>
        <el-button type="primary" :loading="loading" @click="reload">刷新</el-button>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert
      title="支持 Markdown + LaTeX：行内公式用 $...$，块级公式用 $$...$$；下方预览可直接查看效果。"
      type="info"
      :closable="false"
      show-icon
      class="mb"
    />

    <el-form label-position="top">
      <el-form-item label="标题"><el-input v-model="title" /></el-form-item>
      <el-form-item label="内容"><el-input v-model="content" type="textarea" :rows="4" /></el-form-item>
      <el-form-item><el-checkbox v-model="pinned">置顶</el-checkbox></el-form-item>
      <el-form-item><el-button type="primary" :loading="publishing" @click="publish">发布公告</el-button></el-form-item>
    </el-form>
    <el-card v-if="content.trim()" class="preview-card">
      <template #header>
        <div class="section-inline-title">
          <el-icon><Document /></el-icon>
          <span>Markdown 预览</span>
        </div>
      </template>
      <div class="md-body" v-html="renderMarkdown(content)" />
    </el-card>

    <el-table :data="rows" stripe>
      <el-table-column prop="announcement_id" label="ID" width="80" />
      <el-table-column prop="title" label="标题" width="220" />
      <el-table-column prop="pinned" label="置顶" width="80">
        <template #default="{row}">{{ row.pinned ? '是' : '否' }}</template>
      </el-table-column>
      <el-table-column prop="created_by" label="发布人" width="120" />
      <el-table-column prop="created_at" label="发布时间" width="180" :formatter="tableTimeFormatter" />
      <el-table-column label="内容" min-width="260">
        <template #default="{ row }">
          <div class="md-body" v-html="renderMarkdown(row.content)" />
        </template>
      </el-table-column>
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
import { renderMarkdown } from "../../lib/markdown";
import { Bell, Document } from "@element-plus/icons-vue";
import { formatServerDateTime } from "../../lib/time";

const loading = ref(false);
const publishing = ref(false);
const error = ref("");
const rows = ref<Announcement[]>([]);
const title = ref("");
const content = ref("");
const pinned = ref(false);

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

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
.section-title-wrap { display: inline-flex; align-items: center; gap: 10px; }
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
.section-inline-title { display: inline-flex; align-items: center; gap: 8px; }
.title { font-weight: 700; }
.sub { color:#64748b; font-size:12px; }
.mb { margin-bottom: 12px; }
.preview-card { margin-bottom: 12px; }
.md-body :deep(p) { margin: 6px 0; }
.md-body :deep(h1), .md-body :deep(h2), .md-body :deep(h3), .md-body :deep(h4) { margin: 8px 0; }
.md-body :deep(ul) { padding-left: 18px; margin: 6px 0; }
.md-body :deep(code) { background: #f1f5f9; padding: 1px 4px; border-radius: 4px; }
.md-body :deep(blockquote) { margin: 6px 0; padding-left: 10px; color: #475569; border-left: 3px solid #cbd5e1; }
</style>
