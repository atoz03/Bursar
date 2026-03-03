<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><Document /></el-icon></span>
          <div>
          <div class="title">用户准则管理</div>
          <div class="sub">注册页与用户端“公告与用户准则”页面都会展示这里的内容</div>
          </div>
        </div>
        <el-button type="primary" :loading="loading" @click="reload">刷新</el-button>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />

    <el-form label-position="top">
      <el-form-item label="用户准则（Markdown）">
        <el-input v-model="content" type="textarea" :rows="14" placeholder="请输入用户准则内容" />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :loading="saving" @click="save">保存用户准则</el-button>
      </el-form-item>
    </el-form>

    <el-card v-if="content.trim()" class="preview-card">
      <template #header>
        <div class="section-inline-title">
          <el-icon><View /></el-icon>
          <span>预览</span>
        </div>
      </template>
      <div class="md-body" v-html="renderMarkdown(content)" />
    </el-card>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { renderMarkdown } from "../../lib/markdown";
import { Document, View } from "@element-plus/icons-vue";

const loading = ref(false);
const saving = ref(false);
const error = ref("");
const success = ref("");
const content = ref("");

async function reload() {
  loading.value = true;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminGuideline();
    content.value = String(r.content || "").trim();
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  } finally {
    loading.value = false;
  }
}

async function save() {
  const text = String(content.value || "").trim();
  if (!text) {
    error.value = "用户准则不能为空";
    return;
  }
  saving.value = true;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminSetGuideline(text);
    success.value = "用户准则已保存";
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  } finally {
    saving.value = false;
  }
}

reload();
</script>

<style scoped>
.row { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
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
.preview-card { margin-top: 12px; }
.md-body :deep(p) { margin: 6px 0; }
.md-body :deep(h1), .md-body :deep(h2), .md-body :deep(h3), .md-body :deep(h4) { margin: 8px 0; }
.md-body :deep(ul), .md-body :deep(ol) { padding-left: 18px; margin: 6px 0; }
.md-body :deep(code) { background: #f1f5f9; padding: 1px 4px; border-radius: 4px; }
.md-body :deep(blockquote) { margin: 6px 0; padding-left: 10px; color: #475569; border-left: 3px solid #cbd5e1; }
</style>
