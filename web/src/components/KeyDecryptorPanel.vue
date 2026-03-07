<template>
  <div class="decryptor-panel">
    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />

    <el-form label-position="top">
      <el-form-item label="加密密钥串">
        <el-input
          v-model="payload"
          type="textarea"
          :rows="8"
          placeholder="粘贴整段加密密钥串（base64url）"
        />
      </el-form-item>
      <el-form-item label="提取码">
        <el-input v-model="code" placeholder="例如 A1B2-C3D4-E5F6" />
      </el-form-item>
    </el-form>

    <div class="actions">
      <el-button type="primary" :loading="decrypting" @click="decryptNow">解密</el-button>
    </div>

    <template v-if="decryptedKey">
      <el-divider />
      <el-descriptions :column="2" border>
        <el-descriptions-item label="节点编号">{{ env.node_id || "-" }}</el-descriptions-item>
        <el-descriptions-item label="节点账号">{{ env.local_username || "-" }}</el-descriptions-item>
        <el-descriptions-item label="平台账号">{{ env.billing_username || "-" }}</el-descriptions-item>
        <el-descriptions-item label="SSH 地址">{{ sshHost }}:{{ sshPort }}</el-descriptions-item>
        <el-descriptions-item label="建议文件名">{{ fileName }}</el-descriptions-item>
        <el-descriptions-item label="签发时间">{{ formatServerDateTime(env.issued_at || "") }}</el-descriptions-item>
      </el-descriptions>

      <el-alert
        :title="`连接前请先执行：chmod 600 ${fileName}`"
        type="info"
        :closable="false"
        show-icon
        class="mb"
        style="margin-top: 12px"
      />
      <el-form-item label="SSH 连接命令">
        <el-input :model-value="sshCommand" readonly />
      </el-form-item>
      <el-form-item label="解密后的私钥（可选查看）">
        <el-input :model-value="decryptedKey" type="textarea" :rows="7" readonly />
      </el-form-item>
      <div class="actions">
        <el-button type="success" @click="downloadKey">下载私钥文件（txt）</el-button>
        <el-button @click="copySSHCommand">复制 SSH 命令</el-button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { ApiClient } from "../lib/api";
import { writeClipboardText } from "../lib/clipboard";
import { settingsState } from "../lib/settingsStore";
import { formatServerDateTime } from "../lib/time";

type KeyEnvelope = {
  v: string;
  alg: string;
  salt?: string;
  nonce?: string;
  ciphertext?: string;
  age_ciphertext?: string;
  node_id: string;
  local_username: string;
  billing_username: string;
  ssh_host: string;
  ssh_port: number;
  file_name: string;
  issued_at: string;
};

const props = defineProps<{
  initialPayload?: string;
  initialCode?: string;
}>();

const payload = ref("");
const code = ref("");
const decrypting = ref(false);
const error = ref("");
const success = ref("");
const env = ref<KeyEnvelope>({
  v: "",
  alg: "",
  salt: "",
  nonce: "",
  ciphertext: "",
  node_id: "",
  local_username: "",
  billing_username: "",
  ssh_host: "",
  ssh_port: 22,
  file_name: "",
  issued_at: "",
});
const decryptedKey = ref("");

watch(
  () => props.initialPayload,
  (v) => {
    payload.value = String(v || "");
  },
  { immediate: true },
);
watch(
  () => props.initialCode,
  (v) => {
    code.value = String(v || "");
  },
  { immediate: true },
);

const sshHost = computed(() => String(env.value.ssh_host || "").trim() || "<请向管理员确认节点IP>");
const sshPort = computed(() => {
  const n = Number(env.value.ssh_port || 0);
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : 22;
});
const fileName = computed(() => {
  const v = String(env.value.file_name || "").trim();
  if (v) return v;
  const local = String(env.value.local_username || "").trim() || "id_ed25519";
  return `${sshPort.value}_${local}.txt`;
});
const sshCommand = computed(() => {
  const local = String(env.value.local_username || "").trim() || "<用户名>";
  return `ssh -i ${fileName.value} ${local}@${sshHost.value} -p ${sshPort.value}`;
});

function normalizeCode(raw: string): string {
  return String(raw || "")
    .toUpperCase()
    .replace(/[^A-Z0-9]/g, "");
}

function base64UrlToBytes(input: string): Uint8Array {
  const s = String(input || "").trim().replace(/-/g, "+").replace(/_/g, "/");
  const pad = s.length % 4 === 0 ? "" : "=".repeat(4 - (s.length % 4));
  const bin = atob(s + pad);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i += 1) out[i] = bin.charCodeAt(i);
  return out;
}

function hexToBytes(hex: string): Uint8Array {
  const text = String(hex || "").trim().toLowerCase();
  if (!/^[0-9a-f]+$/.test(text) || text.length % 2 !== 0) {
    throw new Error("加密串格式不正确（hex 字段非法）");
  }
  const out = new Uint8Array(text.length / 2);
  for (let i = 0; i < out.length; i += 1) {
    out[i] = parseInt(text.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(out).set(bytes);
  return out;
}

function parseEnvelopeFromPayload(rawPayload: string): KeyEnvelope {
  const rawEnv = base64UrlToBytes(String(rawPayload || "").trim());
  const envText = new TextDecoder().decode(rawEnv);
  return JSON.parse(envText) as KeyEnvelope;
}

async function decryptViaServer(normalizedCode: string) {
  const client = new ApiClient(settingsState.baseUrl);
  const r = await client.decryptProvisionKey(payload.value.trim(), normalizedCode);
  const parsed = r.envelope as KeyEnvelope;
  const plain = String(r.private_key || "");
  if (!plain.includes("BEGIN OPENSSH PRIVATE KEY") || !plain.includes("END OPENSSH PRIVATE KEY")) {
    throw new Error("服务端解密结果不是有效私钥，请联系管理员重置");
  }
  env.value = parsed;
  decryptedKey.value = plain.trim() + "\n";
  success.value = "解密成功，已可下载私钥文件并按 SSH 命令连接。";
}

async function decryptNow() {
  error.value = "";
  success.value = "";
  decryptedKey.value = "";
  if (!payload.value.trim()) {
    error.value = "请先粘贴加密密钥串";
    return;
  }
  const normalized = normalizeCode(code.value);
  if (!normalized) {
    error.value = "请先输入提取码";
    return;
  }
  decrypting.value = true;
  try {
    const parsed = parseEnvelopeFromPayload(payload.value.trim());
    const isAgeV2 = String(parsed.v || "").trim() === "gpuops-key-v2" || String(parsed.alg || "").toLowerCase().includes("age");
    if (isAgeV2) {
      await decryptViaServer(normalized);
      return;
    }
    if (!window.crypto?.subtle) {
      await decryptViaServer(normalized);
      return;
    }
    if (String(parsed.v || "").trim() !== "gpuops-key-v1") {
      throw new Error("加密串版本不支持，请联系管理员重新开通");
    }
    const saltHex = String(parsed.salt || "").trim().toLowerCase();
    const material = `gpuops-provision-key-v1|${normalized}|${saltHex}`;
    const digest = await window.crypto.subtle.digest("SHA-256", new TextEncoder().encode(material));
    const aesKey = await window.crypto.subtle.importKey("raw", digest, { name: "AES-GCM" }, false, ["decrypt"]);

    const nonce = toArrayBuffer(hexToBytes(String(parsed.nonce || "")));
    const ciphertext = toArrayBuffer(hexToBytes(String(parsed.ciphertext || "")));
    const plainBuf = await window.crypto.subtle.decrypt({ name: "AES-GCM", iv: nonce }, aesKey, ciphertext);
    const plain = new TextDecoder().decode(new Uint8Array(plainBuf));
    if (!plain.includes("BEGIN OPENSSH PRIVATE KEY") || !plain.includes("END OPENSSH PRIVATE KEY")) {
      throw new Error("解密结果不是有效私钥，请确认提取码是否正确");
    }

    env.value = parsed;
    decryptedKey.value = plain.trim() + "\n";
    success.value = "解密成功，已可下载私钥文件并按 SSH 命令连接。";
  } catch (e: any) {
    const msg = e?.message ? String(e.message) : "";
    if (window.crypto?.subtle && /not.?supported|notsupported|webcrypto|subtle/i.test(msg.toLowerCase())) {
      try {
        await decryptViaServer(normalized);
        return;
      } catch (e2: any) {
        error.value = e2?.message ? String(e2.message) : "解密失败，请检查加密串和提取码";
        return;
      }
    }
    error.value = e?.message ? String(e.message) : "解密失败，请检查加密串和提取码";
  } finally {
    decrypting.value = false;
  }
}

function downloadKey() {
  if (!decryptedKey.value) {
    error.value = "请先完成解密";
    return;
  }
  const blob = new Blob([decryptedKey.value], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = fileName.value;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
  ElMessage.success(`已下载 ${fileName.value}`);
}

async function copySSHCommand() {
  try {
    await writeClipboardText(sshCommand.value);
    ElMessage.success("SSH 命令已复制");
  } catch {
    ElMessage.error("复制失败，请手动复制");
  }
}
</script>

<style scoped>
.decryptor-panel .mb {
  margin-bottom: 12px;
}
.decryptor-panel .actions {
  display: flex;
  gap: 10px;
  align-items: center;
}
</style>
