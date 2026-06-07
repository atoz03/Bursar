<template>
  <div class="admin-account-provision-page">
    <el-card class="section-card overview-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-provision"><el-icon><Key /></el-icon></span>
            <span>节点账号开通</span>
          </div>
          <div class="head-actions">
            <el-button :loading="pageLoading" @click="reloadAll">刷新</el-button>
          </div>
        </div>
      </template>
      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
      <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
      <el-alert
        title="本页专门处理节点账号开通：包含“开通申请审核”、“开通执行”和“开通历史”。账号映射的增删改查已单独保留在“账号映射”页面。"
        type="info"
        show-icon
        :closable="false"
      />
    </el-card>

    <el-card class="section-card provision-card">
      <template #header>
        <div class="section-title-wrap provision-head">
          <span class="section-icon tone-provision"><el-icon><Key /></el-icon></span>
          <span>节点账号开通（密文进平台内，提取码走邮箱）</span>
        </div>
      </template>
      <el-form label-position="top">
        <el-row :gutter="12">
          <el-col :span="8">
            <el-form-item label="平台账号">
              <el-autocomplete
                v-model="provisionForm.billing_username"
                style="width: 100%"
                clearable
                placeholder="输入平台账号"
                :fetch-suggestions="queryBillingOptions"
                @select="onProvisionBillingSelect"
                @change="onProvisionBillingChange"
                @blur="onProvisionBillingBlur"
              />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="节点编号">
              <el-autocomplete
                v-model="provisionForm.node_id"
                style="width: 100%"
                clearable
                placeholder="例如 60000"
                :fetch-suggestions="queryNodeOptions"
                @change="onProvisionNodeChange"
                @blur="onProvisionNodeBlur"
                @select="onProvisionNodeSelect"
              >
                <template #default="{ item }">
                  <div class="node-option-item">
                    <div class="node-option-title">{{ item.value }}</div>
                    <div class="node-option-desc">{{ item.description }}</div>
                  </div>
                </template>
              </el-autocomplete>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="节点账号">
              <el-input v-model="provisionForm.local_username" placeholder="例如 zhangsan" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="12">
            <el-form-item label="SSH 主机地址（可选）">
              <el-input
                v-model="provisionForm.ssh_host"
                placeholder="默认 controller.example.org（可修改，系统会记住上次使用地址）"
                @blur="rememberProvisionSSHHostFromForm"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="SSH 端口（可选）">
              <el-input-number v-model="provisionForm.ssh_port" :min="0" :max="65535" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
      <div class="provision-node-preview">
        <div class="preview-head">
          <div class="section-title-wrap">
            <span class="section-icon tone-node"><el-icon><Monitor /></el-icon></span>
            <span>已选节点核对信息</span>
          </div>
        </div>
        <el-empty
          v-if="!provisionForm.node_id.trim()"
          description="请选择节点后自动展示 CPU / GPU / 硬盘 信息，避免开通到错误节点"
          :image-size="58"
        />
        <el-alert
          v-else-if="!selectedProvisionNode"
          title="未找到该节点的最新上报信息，请检查节点编号是否正确，或先到“节点状态”页确认该节点已接入。"
          type="warning"
          show-icon
          :closable="false"
          class="mb"
        />
        <el-descriptions v-else :column="3" border size="small" class="preview-desc">
          <el-descriptions-item label="节点编号">{{ selectedProvisionNode.node_id }}</el-descriptions-item>
          <el-descriptions-item label="节点IP">{{ selectedProvisionNode.node_ip || "-" }}</el-descriptions-item>
          <el-descriptions-item label="最后心跳">{{ fmtTime(selectedProvisionNode.last_seen_at) }}</el-descriptions-item>
          <el-descriptions-item label="CPU 型号">{{ selectedProvisionNode.cpu_model || "-" }}</el-descriptions-item>
          <el-descriptions-item label="CPU 数量">{{ fmtNodeCount(selectedProvisionNode.cpu_count, "核") }}</el-descriptions-item>
          <el-descriptions-item label="GPU 型号">{{ selectedProvisionNode.gpu_model || "-" }}</el-descriptions-item>
          <el-descriptions-item label="GPU 数量">{{ fmtNodeCount(selectedProvisionNode.gpu_count, "张") }}</el-descriptions-item>
          <el-descriptions-item label="硬盘总量">{{ fmtNodeGB(selectedProvisionNode.disk_total_gb) }}</el-descriptions-item>
          <el-descriptions-item label="硬盘已用">{{ fmtNodeGB(selectedProvisionNode.disk_used_gb) }}</el-descriptions-item>
        </el-descriptions>
      </div>
      <div class="provision-user-preview">
        <div class="preview-head">
          <div class="section-title-wrap">
            <span class="section-icon tone-user"><el-icon><UserFilled /></el-icon></span>
            <span>已选平台账号核对信息</span>
          </div>
          <el-button v-if="provisionUserDetail?.username" link type="primary" @click="openProfile(provisionUserDetail.username)">
            打开完整详情
          </el-button>
        </div>
        <el-skeleton v-if="provisionUserLoading" :rows="2" animated />
        <el-alert
          v-else-if="provisionUserError"
          :title="provisionUserError"
          type="error"
          show-icon
          :closable="false"
          class="mb"
        />
        <el-empty v-else-if="!provisionUserDetail" description="请选择平台账号后自动展示详情，避免开通错误" :image-size="58" />
        <el-descriptions v-else :column="3" border size="small" class="preview-desc">
          <el-descriptions-item label="平台账号">{{ provisionUserDetail.username }}</el-descriptions-item>
          <el-descriptions-item label="用户级别">{{ roleText(provisionUserDetail.role) }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="statusTagType(provisionUserDetail.status)" size="small">{{ statusText(provisionUserDetail.status) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="真实姓名">{{ provisionUserDetail.real_name || "-" }}</el-descriptions-item>
          <el-descriptions-item label="学号">{{ provisionUserDetail.student_id || "-" }}</el-descriptions-item>
          <el-descriptions-item label="邮箱">{{ provisionUserDetail.email || "-" }}</el-descriptions-item>
          <el-descriptions-item label="通用积分">{{ fmt2(provisionUserDetail.general_balance ?? provisionUserDetail.balance ?? 0) }}</el-descriptions-item>
          <el-descriptions-item label="专属积分">{{ fmt2(provisionUserDetail.exclusive_balance ?? 0) }}</el-descriptions-item>
          <el-descriptions-item label="总积分">{{ fmt2(provisionUserDetail.total_balance ?? ((provisionUserDetail.general_balance ?? provisionUserDetail.balance ?? 0) + (provisionUserDetail.exclusive_balance ?? 0))) }}</el-descriptions-item>
          <el-descriptions-item label="导师">{{ provisionUserDetail.advisor || "-" }}</el-descriptions-item>
          <el-descriptions-item label="预计毕业">{{ fmtGrad(provisionUserDetail.expected_graduation_year, provisionUserDetail.expected_graduation_month) }}</el-descriptions-item>
          <el-descriptions-item label="已有映射">{{ (provisionUserDetail.node_accounts || []).length }} 条</el-descriptions-item>
        </el-descriptions>
      </div>
      <el-alert
        title="若节点上已存在同名账号，系统会复用该账号并刷新 authorized_keys；若用户丢失密钥，可在冲突提示后选择“重新生成新密钥并重发”。系统默认生成 ed25519 密钥，密文发送到平台内通知，提取码单独邮件发送。"
        type="warning"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-alert v-if="provisionActionError" :title="provisionActionError" type="error" show-icon class="mb" />
      <el-alert v-if="provisionActionSuccess" :title="provisionActionSuccess" type="success" show-icon class="mb" />
      <div class="provision-actions">
        <el-button type="success" :loading="provisioning" @click="provisionAccount">开通账号并发送提取码邮件</el-button>
      </div>
    </el-card>

    <el-card class="section-card mapping-review-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-note"><el-icon><Document /></el-icon></span>
            <span>节点账号开通申请审核</span>
            <el-badge
              :value="pendingOpenRequestCount"
              :hidden="pendingOpenRequestCount <= 0"
              type="danger"
              class="pending-count-badge"
            />
          </div>
          <div class="head-actions">
            <el-select v-model="openRequestStatus" size="small" style="width: 150px" @change="reloadOpenRequests">
              <el-option label="待处理" value="pending" />
              <el-option label="已处理" value="approved" />
              <el-option label="已拒绝" value="rejected" />
              <el-option label="全部" value="" />
            </el-select>
            <el-button size="small" plain @click="openOpenRejectHistoryDialog">驳回历史</el-button>
            <el-button size="small" :loading="openRequestsLoading" @click="reloadOpenRequests">刷新</el-button>
          </div>
        </div>
      </template>
      <el-alert
        title="这里集中展示“节点账号开通申请”。用户自助发起的绑定 challenge 不在这里审核。"
        type="info"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-alert v-if="pendingOpenRequestCount > 0" type="warning" :closable="false" show-icon class="mb pending-open-banner">
        <template #title>{{ pendingAlertTitle }}</template>
        <div class="pending-open-actions">
          <el-button size="small" type="warning" plain @click="focusPendingRequests">只看待处理</el-button>
          <el-button size="small" @click="reloadOpenRequests">立即刷新</el-button>
        </div>
      </el-alert>
      <el-table :data="openRequests" stripe size="small" max-height="420">
        <el-table-column type="expand" width="44">
          <template #default="{ row }">
            <el-descriptions v-if="findApplicant(row.billing_username)" :column="3" border size="small" class="note-user-detail">
              <el-descriptions-item label="平台账号">{{ findApplicant(row.billing_username)?.username }}</el-descriptions-item>
              <el-descriptions-item label="用户级别">{{ roleText(findApplicant(row.billing_username)?.role || "") }}</el-descriptions-item>
              <el-descriptions-item label="账号状态">
                <el-tag :type="statusTagType(findApplicant(row.billing_username)?.status || '')" size="small">
                  {{ statusText(findApplicant(row.billing_username)?.status || "") }}
                </el-tag>
              </el-descriptions-item>
              <el-descriptions-item label="真实姓名">{{ findApplicant(row.billing_username)?.real_name || "-" }}</el-descriptions-item>
              <el-descriptions-item label="学号">{{ findApplicant(row.billing_username)?.student_id || "-" }}</el-descriptions-item>
              <el-descriptions-item label="邮箱">{{ findApplicant(row.billing_username)?.email || "-" }}</el-descriptions-item>
              <el-descriptions-item label="电话">{{ findApplicant(row.billing_username)?.phone || "-" }}</el-descriptions-item>
              <el-descriptions-item label="导师">{{ findApplicant(row.billing_username)?.advisor || "-" }}</el-descriptions-item>
              <el-descriptions-item label="预计毕业">{{ fmtGrad(findApplicant(row.billing_username)?.expected_graduation_year || 0, findApplicant(row.billing_username)?.expected_graduation_month || 0) }}</el-descriptions-item>
              <el-descriptions-item label="通用积分">{{ fmt2(findApplicant(row.billing_username)?.balance || 0) }}</el-descriptions-item>
              <el-descriptions-item label="专属积分">{{ fmt2(findApplicant(row.billing_username)?.exclusive_balance || 0) }}</el-descriptions-item>
              <el-descriptions-item label="总积分">{{ fmt2(findApplicant(row.billing_username)?.total_balance || 0) }}</el-descriptions-item>
            </el-descriptions>
            <el-alert
              v-else
              title="本地缓存未命中申请人信息，请点击平台账号查看完整详情。"
              type="info"
              :closable="false"
              show-icon
            />
          </template>
        </el-table-column>
        <el-table-column prop="request_id" label="申请ID" width="86" />
        <el-table-column label="类型" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="requestTypeTagType(row.request_type)">{{ requestTypeText(row.request_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="requestStatusTagType(row.status)" size="small">{{ requestStatusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="申请人" width="170">
          <template #default="{ row }">
            <el-button link type="primary" @click="openProfile(row.billing_username)">{{ row.billing_username }}</el-button>
          </template>
        </el-table-column>
        <el-table-column label="申请人信息" min-width="360">
          <template #default="{ row }">
            <div class="note-user-lines">
              <div class="line-1">{{ applicantSummary(row.billing_username) }}</div>
              <div class="line-2">{{ applicantContact(row.billing_username) }}</div>
              <div class="line-3">{{ applicantAdvisor(row.billing_username) }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="message" label="申请说明" min-width="300" />
        <el-table-column label="申请时间" min-width="170">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="审核信息" min-width="180">
          <template #default="{ row }">
            <div class="mini">审核人：{{ row.reviewed_by || "-" }}</div>
            <div class="mini">审核时间：{{ fmtTime(row.reviewed_at || "") }}</div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="320" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button v-if="String(row.request_type || '') === 'open'" size="small" @click="applyOpenRequestToProvision(row)">带入开通</el-button>
              <el-button
                v-if="row.status === 'pending'"
                size="small"
                type="success"
                :loading="openRequestActionLoadingId === row.request_id"
                @click="markOpenRequestProcessed(row)"
              >
                标记已处理
              </el-button>
              <el-button
                v-if="row.status === 'pending'"
                size="small"
                type="danger"
                plain
                :loading="openRequestActionLoadingId === row.request_id"
                @click="markOpenRequestRejected(row)"
              >
                拒绝
              </el-button>
              <el-button
                v-else
                size="small"
                type="warning"
                plain
                :loading="openRequestActionLoadingId === row.request_id"
                @click="markOpenRequestPending(row)"
              >
                恢复待处理
              </el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="section-card provision-history-card">
      <template #header>
        <div class="head-actions section-title-wrap provision-history-head">
          <span class="section-icon tone-history"><el-icon><Clock /></el-icon></span>
          <span>节点账号开通历史</span>
          <el-button size="small" :loading="provisionLogsLoading" @click="reloadProvisionLogs">刷新历史</el-button>
        </div>
      </template>
      <el-table :data="provisionLogs" stripe size="small" max-height="320">
        <el-table-column label="开通时间" min-width="172" align="left" header-align="left">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="created_by" label="操作人" width="120" align="left" header-align="left" />
        <el-table-column prop="billing_username" label="平台账号" width="160" align="left" header-align="left" />
        <el-table-column prop="node_id" label="节点编号" width="120" align="left" header-align="left" />
        <el-table-column prop="local_username" label="节点账号" width="150" align="left" header-align="left" />
        <el-table-column prop="email" label="邮箱" min-width="220" align="left" header-align="left" />
        <el-table-column label="邮件状态" width="100" align="left" header-align="left">
          <template #default="{ row }">
            <el-tag :type="row.mail_sent ? 'success' : 'danger'" size="small">{{ row.mail_sent ? "成功" : "失败" }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="mail_error" label="失败原因" min-width="220" align="left" header-align="left" />
      </el-table>
    </el-card>

    <el-dialog v-model="openRejectHistoryVisible" title="开通申请驳回历史" width="1050px" destroy-on-close>
      <div class="head-actions mb">
        <el-button size="small" :loading="openRejectHistoryLoading" @click="reloadOpenRejectHistory">刷新</el-button>
      </div>
      <el-table :data="openRejectHistoryRows" stripe size="small" max-height="460" empty-text="暂无驳回记录">
        <el-table-column prop="request_id" label="申请ID" width="90" />
        <el-table-column label="类型" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="requestTypeTagType(row.request_type)">{{ requestTypeText(row.request_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="平台账号" width="170">
          <template #default="{ row }">
            <el-button link type="primary" @click="openProfile(row.billing_username)">{{ row.billing_username }}</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="node_id" label="节点编号" width="130" />
        <el-table-column prop="local_username" label="节点账号" width="150" />
        <el-table-column prop="message" label="申请说明/备注" min-width="260" />
        <el-table-column prop="reject_reason" label="拒绝理由" min-width="240" />
        <el-table-column label="审核信息" min-width="220">
          <template #default="{ row }">
            <div class="mini">审核人：{{ row.reviewed_by || "-" }}</div>
            <div class="mini">审核时间：{{ fmtTime(row.reviewed_at || "") }}</div>
            <div class="mini">提交时间：{{ fmtTime(row.created_at || "") }}</div>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="openRejectHistoryVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="provisionResultVisible" title="节点账号开通结果" width="900px" destroy-on-close>
      <template v-if="provisionResult">
        <el-alert
          :title="provisionResult.mail_sent ? '平台内已下发密文通知，提取码已发邮箱。' : '平台内密文通知已下发，但提取码邮件发送失败，请手动通知用户提取码。'"
          :type="provisionResult.mail_sent ? 'success' : 'warning'"
          :closable="false"
          show-icon
          class="mb"
        />
        <el-alert v-if="provisionResult.mail_error" :title="provisionResult.mail_error" type="error" show-icon class="mb" />
        <el-alert v-if="provisionResult.log_error" :title="provisionResult.log_error" type="warning" show-icon class="mb" />
        <el-alert v-if="provisionResult.notice_error" :title="provisionResult.notice_error" type="error" show-icon class="mb" />
        <el-descriptions :column="2" border>
          <el-descriptions-item label="平台账号">{{ provisionResult.billing_username }}</el-descriptions-item>
          <el-descriptions-item label="通知邮箱">{{ provisionResult.email }}</el-descriptions-item>
          <el-descriptions-item label="节点编号">{{ provisionResult.node_id }}</el-descriptions-item>
          <el-descriptions-item label="节点账号">{{ provisionResult.local_username }}</el-descriptions-item>
          <el-descriptions-item label="建议文件名">{{ provisionResult.download_filename }}</el-descriptions-item>
          <el-descriptions-item label="SSH 地址">{{ provisionResult.ssh_host }}:{{ provisionResult.ssh_port }}</el-descriptions-item>
        </el-descriptions>
        <div class="kv-row">
          <span class="kv-label">提取码</span>
          <el-input :model-value="provisionResult.decrypt_code" readonly />
          <el-button @click="copyText(provisionResult.decrypt_code)">复制</el-button>
        </div>
        <div class="kv-row">
          <span class="kv-label">去解密入口地址</span>
          <el-input :model-value="provisionResult.decrypt_url" readonly />
          <el-button @click="copyText(provisionResult.decrypt_url)">复制</el-button>
        </div>
        <div class="kv-row">
          <span class="kv-label">SSH 连接命令</span>
          <el-input :model-value="provisionResult.ssh_command" readonly />
          <el-button @click="copyText(provisionResult.ssh_command)">复制</el-button>
        </div>
        <el-form-item label="加密密钥串（完整复制）">
          <el-input :model-value="provisionResult.encrypted_payload" type="textarea" :rows="7" readonly />
          <div class="payload-actions">
            <el-button @click="copyText(provisionResult.encrypted_payload)">复制密文（应发平台内，不发邮箱）</el-button>
          </div>
        </el-form-item>
      </template>
      <template #footer>
        <el-button @click="provisionResultVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  ApiClient,
  type AdminAccountProvisionLog,
  type AdminAccountProvisionResp,
  type AdminUserDetail,
  type NodeStatus,
  type PlatformUserDetail,
  type UserNodeAccount,
  type UserRequest,
} from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime, toServerEpochMs } from "../../lib/time";
import { writeClipboardText } from "../../lib/clipboard";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { Clock, Document, Key, Monitor, UserFilled } from "@element-plus/icons-vue";

const pageLoading = ref(false);
const provisioning = ref(false);
const error = ref("");
const success = ref("");
const billingOptions = ref<string[]>([]);
type ProvisionNodeOption = {
  value: string;
  description: string;
  keywords: string;
};

const nodeOptions = ref<ProvisionNodeOption[]>([]);
const nodeDetailByID = ref<Record<string, NodeStatus>>({});
const nodeIPByID = ref<Record<string, string>>({});
const platformUsers = ref<AdminUserDetail[]>([]);
const profileVisible = ref(false);
const selectedProfileUsername = ref("");

const provisionResultVisible = ref(false);
const provisionResult = ref<AdminAccountProvisionResp | null>(null);
const provisionLogsLoading = ref(false);
const provisionLogs = ref<AdminAccountProvisionLog[]>([]);
const openRequestsLoading = ref(false);
const openRequests = ref<UserRequest[]>([]);
const openRejectHistoryVisible = ref(false);
const openRejectHistoryLoading = ref(false);
const openRejectHistoryRows = ref<UserRequest[]>([]);
const openRequestStatus = ref<"pending" | "approved" | "rejected" | "">("pending");
const openRequestActionLoadingId = ref(0);
const pendingOpenRequests = ref<UserRequest[]>([]);
const provisionUserLoading = ref(false);
const provisionUserError = ref("");
const provisionActionError = ref("");
const provisionActionSuccess = ref("");
const provisionUserDetail = ref<PlatformUserDetail | null>(null);
const provisionUserLastFetched = ref("");
let provisionUserFetchSeq = 0;

const DEFAULT_PROVISION_SSH_HOST = "controller.example.org";
const PROVISION_SSH_HOST_STORAGE_KEY = "gpuops.provision.last_ssh_host";

const provisionForm = reactive({
  billing_username: "",
  node_id: "",
  local_username: "",
  ssh_host: loadProvisionSSHHostDefault(),
  ssh_port: 0,
});

const pendingOpenRequestCount = computed(() => pendingOpenRequests.value.length);
const pendingOpenRequestUsersText = computed(() => {
  const users = uniqSorted(pendingOpenRequests.value.map((x) => String(x.billing_username || "").trim()).filter(Boolean));
  if (!users.length) return "";
  const names = users.slice(0, 4).join("、");
  return users.length > 4 ? `${names} 等 ${users.length} 人` : names;
});
const pendingAlertTitle = computed(() => {
  if (pendingOpenRequestCount.value <= 0) return "";
  const base = `当前有 ${pendingOpenRequestCount.value} 条待处理的节点账号开通申请`;
  const users = pendingOpenRequestUsersText.value;
  return users ? `${base}（申请人：${users}）` : base;
});
const selectedProvisionNode = computed<NodeStatus | null>(() => {
  const nodeID = String(provisionForm.node_id || "").trim();
  if (!nodeID) return null;
  return nodeDetailByID.value[nodeID] || null;
});

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

function loadProvisionSSHHostDefault(): string {
  try {
    const v = String(localStorage.getItem(PROVISION_SSH_HOST_STORAGE_KEY) || "").trim();
    return v || DEFAULT_PROVISION_SSH_HOST;
  } catch {
    return DEFAULT_PROVISION_SSH_HOST;
  }
}

function rememberProvisionSSHHost(host: string) {
  const v = String(host || "").trim();
  if (!v) return;
  try {
    localStorage.setItem(PROVISION_SSH_HOST_STORAGE_KEY, v);
  } catch {
    // ignore storage errors
  }
}

function rememberProvisionSSHHostFromForm() {
  rememberProvisionSSHHost(provisionForm.ssh_host);
}

function uniqSorted(items: string[]): string[] {
  const s = new Set<string>();
  for (const item of items) {
    const v = String(item || "").trim();
    if (v) s.add(v);
  }
  return Array.from(s).sort((a, b) => a.localeCompare(b));
}

function fmtTime(v: string): string {
  return formatServerDateTime(v);
}

function fmt2(v: number): string {
  return Number(v || 0).toFixed(2);
}

function fmtNodeCount(v?: number, unit = ""): string {
  const n = Number(v ?? 0);
  if (!Number.isFinite(n) || n <= 0) return "-";
  return `${n}${unit}`;
}

function fmtNodeGB(v?: number): string {
  const n = Number(v ?? 0);
  if (!Number.isFinite(n) || n <= 0) return "-";
  return `${n.toFixed(n >= 100 ? 0 : 1)} GB`;
}

function fmtGrad(year: number, month: number): string {
  if (!year || !month) return "-";
  return `${year}-${String(month).padStart(2, "0")}`;
}

function roleText(role: string): string {
  const r = String(role || "").trim();
  if (r === "admin") return "管理员";
  if (r === "power_user") return "高级用户";
  return "普通用户";
}

function statusText(status: string): string {
  const s = String(status || "").trim();
  if (s === "blocked") return "已封禁";
  if (s === "limited") return "受限";
  if (s === "warning") return "告警";
  return "正常";
}

function statusTagType(status: string): "success" | "warning" | "danger" | "info" {
  const s = String(status || "").trim();
  if (s === "blocked") return "danger";
  if (s === "limited" || s === "warning") return "warning";
  if (s === "normal") return "success";
  return "info";
}

function requestStatusText(status: string): string {
  const s = String(status || "").trim();
  if (s === "approved") return "已处理";
  if (s === "rejected") return "已拒绝";
  if (s === "pending") return "待处理";
  return s || "-";
}

function requestStatusTagType(status: string): "success" | "warning" | "danger" | "info" {
  const s = String(status || "").trim();
  if (s === "approved") return "success";
  if (s === "rejected") return "danger";
  if (s === "pending") return "warning";
  return "info";
}

function requestTypeText(requestType: string): string {
  const t = String(requestType || "").trim();
  if (t === "open") return "开通";
  if (t === "bind") return "绑定";
  if (t === "unbind") return "解绑";
  return t || "-";
}

function requestTypeTagType(requestType: string): "success" | "warning" | "danger" | "info" {
  const t = String(requestType || "").trim();
  if (t === "open") return "success";
  if (t === "bind") return "warning";
  if (t === "unbind") return "danger";
  return "info";
}

function parseNodePort(node: string): number {
  const n = Number(String(node || "").trim());
  if (!Number.isFinite(n) || n <= 0 || n > 65535) return 0;
  return Math.floor(n);
}

function applyProvisionNodeDefaults(node: string) {
  const id = String(node || "").trim();
  if (!id) return;
  const ip = String(nodeIPByID.value[id] || "").trim();
  if (ip && !String(provisionForm.ssh_host || "").trim()) {
    provisionForm.ssh_host = ip;
  }
  if (!provisionForm.ssh_port || provisionForm.ssh_port <= 0) {
    const p = parseNodePort(id);
    provisionForm.ssh_port = p > 0 ? p : 22;
  }
}

function queryOptions(base: string[], queryString: string, cb: (items: Array<{ value: string }>) => void) {
  const q = String(queryString || "").trim().toLowerCase();
  cb(
    base
      .filter((x) => (q ? x.toLowerCase().includes(q) : true))
      .slice(0, 40)
      .map((x) => ({ value: x })),
  );
}

function queryBillingOptions(queryString: string, cb: (items: Array<{ value: string }>) => void) {
  queryOptions(billingOptions.value, queryString, cb);
}

function queryNodeOptions(queryString: string, cb: (items: ProvisionNodeOption[]) => void) {
  const q = String(queryString || "").trim().toLowerCase();
  cb(
    nodeOptions.value
      .filter((item) => (q ? item.keywords.includes(q) : true))
      .slice(0, 40),
  );
}

function openProfile(username: string) {
  selectedProfileUsername.value = String(username || "").trim();
  if (!selectedProfileUsername.value) return;
  profileVisible.value = true;
}

function findApplicant(username: string): AdminUserDetail | null {
  const u = String(username || "").trim();
  if (!u) return null;
  return (platformUsers.value || []).find((x) => String(x.username || "").trim() === u) || null;
}

function applicantSummary(username: string): string {
  const user = findApplicant(username);
  if (!user) return "未命中本地缓存，请点击平台账号查看完整信息";
  const realName = String(user.real_name || "").trim() || "-";
  const studentID = String(user.student_id || "").trim() || "-";
  const role = roleText(user.role);
  return `${realName}｜学号 ${studentID}｜级别 ${role}`;
}

function applicantContact(username: string): string {
  const user = findApplicant(username);
  if (!user) return "联系方式：-";
  const email = String(user.email || "").trim() || "-";
  const phone = String(user.phone || "").trim() || "-";
  return `邮箱 ${email}｜电话 ${phone}`;
}

function applicantAdvisor(username: string): string {
  const user = findApplicant(username);
  if (!user) return "导师/毕业时间：-";
  const advisor = String(user.advisor || "").trim() || "-";
  const grad = fmtGrad(user.expected_graduation_year, user.expected_graduation_month);
  return `导师 ${advisor}｜预计毕业 ${grad}`;
}

function toPlatformUserDetailFromAdminRow(row: AdminUserDetail): PlatformUserDetail {
  return {
    username: row.username,
    platform_uid: row.platform_uid,
    email: row.email,
    real_name: row.real_name,
    student_id: row.student_id,
    advisor: row.advisor,
    expected_graduation_year: row.expected_graduation_year,
    expected_graduation_month: row.expected_graduation_month,
    phone: row.phone,
    role: row.role,
    balance: Number(row.balance || 0),
    general_balance: Number(row.balance || 0),
    exclusive_balance: Number(row.exclusive_balance || 0),
    total_balance: Number(row.total_balance || (Number(row.balance || 0) + Number(row.exclusive_balance || 0))),
    status: row.status,
    created_at: row.created_at,
    node_accounts: row.node_accounts || [],
  };
}

function syncProvisionUserPreviewFromLocal() {
  const username = String(provisionForm.billing_username || "").trim();
  provisionUserError.value = "";
  if (!username) {
    provisionUserDetail.value = null;
    provisionUserLastFetched.value = "";
    return;
  }
  const row = (platformUsers.value || []).find((u) => String(u.username || "").trim() === username);
  if (row) {
    provisionUserDetail.value = toPlatformUserDetailFromAdminRow(row);
    return;
  }
  provisionUserDetail.value = null;
}

async function fetchProvisionUserDetail(force: boolean) {
  const username = String(provisionForm.billing_username || "").trim();
  if (!username) {
    provisionUserError.value = "";
    provisionUserDetail.value = null;
    provisionUserLastFetched.value = "";
    return;
  }
  if (!force && provisionUserLastFetched.value === username && provisionUserDetail.value?.username === username) {
    return;
  }
  const seq = ++provisionUserFetchSeq;
  provisionUserLoading.value = true;
  provisionUserError.value = "";
  try {
    const r = await client().adminPlatformUserDetail(username);
    if (seq !== provisionUserFetchSeq) return;
    provisionUserDetail.value = r.user;
    provisionUserLastFetched.value = username;
  } catch (e: any) {
    if (seq !== provisionUserFetchSeq) return;
    provisionUserError.value = e?.message ?? String(e);
  } finally {
    if (seq === provisionUserFetchSeq) {
      provisionUserLoading.value = false;
    }
  }
}

function onProvisionBillingSelect(item: { value?: string }) {
  provisionForm.billing_username = String(item?.value || "").trim();
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  syncProvisionUserPreviewFromLocal();
  void fetchProvisionUserDetail(true);
}

function onProvisionBillingChange() {
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  syncProvisionUserPreviewFromLocal();
}

function onProvisionBillingBlur() {
  void fetchProvisionUserDetail(false);
}

function onProvisionNodeSelect(item: { value?: string }) {
  provisionForm.node_id = String(item?.value || "").trim();
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  applyProvisionNodeDefaults(provisionForm.node_id);
}

function onProvisionNodeChange() {
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  applyProvisionNodeDefaults(provisionForm.node_id);
}

function onProvisionNodeBlur() {
  applyProvisionNodeDefaults(provisionForm.node_id);
}

function buildProvisionNodeDescription(node: NodeStatus): string {
  const parts = [
    `CPU ${fmtNodeCount(node.cpu_count, "核")}`,
    `GPU ${fmtNodeCount(node.gpu_count, "张")}`,
    `硬盘 ${fmtNodeGB(node.disk_total_gb)}`,
  ].filter((part) => !part.endsWith("-"));
  const ip = String(node.node_ip || "").trim();
  if (ip) parts.push(ip);
  return parts.join(" | ") || "暂无资源摘要";
}

async function loadNodeOptions() {
  const r = await client().adminNodes(3000);
  const items: ProvisionNodeOption[] = [];
  const ipMap: Record<string, string> = {};
  const detailMap: Record<string, NodeStatus> = {};
  for (const n of r.nodes ?? []) {
    const id = String(n.node_id || "").trim();
    if (!id) continue;
    const description = buildProvisionNodeDescription(n);
    items.push({
      value: id,
      description,
      keywords: `${id} ${description} ${String(n.cpu_model || "")} ${String(n.gpu_model || "")}`.toLowerCase(),
    });
    ipMap[id] = String(n.node_ip || "").trim();
    detailMap[id] = n;
  }
  nodeOptions.value = items.sort((a, b) => a.value.localeCompare(b.value, "zh-Hans-CN", { numeric: true, sensitivity: "base" }));
  nodeIPByID.value = ipMap;
  nodeDetailByID.value = detailMap;
}

async function loadPlatformUsers() {
  const r = await client().adminUsersDetails(3000);
  platformUsers.value = r.users ?? [];
  billingOptions.value = uniqSorted((platformUsers.value || []).map((x) => String(x.username || "").trim()));
  syncProvisionUserPreviewFromLocal();
}

async function reloadProvisionLogs() {
  provisionLogsLoading.value = true;
  try {
    const r = await client().adminProvisionLogs({ limit: 500 });
    provisionLogs.value = r.logs ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    provisionLogsLoading.value = false;
  }
}

async function reloadOpenRequests() {
  openRequestsLoading.value = true;
  try {
    const r = await client().adminRequests({ status: openRequestStatus.value || "", limit: 5000 });
    const allCurrent = r.requests ?? [];
    openRequests.value = allCurrent.filter((x) => String(x.request_type || "").trim() === "open");
    if (!openRequestStatus.value || openRequestStatus.value === "pending") {
      pendingOpenRequests.value = allCurrent.filter((x) => String(x.request_type || "").trim() === "open" && String(x.status || "").trim() === "pending");
      return;
    }
    const pendingResp = await client().adminRequests({ status: "pending", limit: 5000 });
    pendingOpenRequests.value = (pendingResp.requests ?? []).filter((x) => String(x.request_type || "").trim() === "open");
  } catch (e: any) {
    pendingOpenRequests.value = [];
    error.value = e?.message ?? String(e);
  } finally {
    openRequestsLoading.value = false;
  }
}

async function reloadOpenRejectHistory() {
  openRejectHistoryLoading.value = true;
  try {
    const r = await client().adminRequests({ status: "rejected", limit: 5000 });
    openRejectHistoryRows.value = (r.requests ?? [])
      .filter((x) => String(x.request_type || "").trim() === "open")
      .sort((a, b) => {
        const ta = toServerEpochMs(String(a.reviewed_at || a.updated_at || a.created_at || ""));
        const tb = toServerEpochMs(String(b.reviewed_at || b.updated_at || b.created_at || ""));
        if (Number.isFinite(tb) && Number.isFinite(ta) && tb !== ta) return tb - ta;
        return Number(b.request_id || 0) - Number(a.request_id || 0);
      });
  } catch (e: any) {
    error.value = e?.message ?? String(e);
    openRejectHistoryRows.value = [];
  } finally {
    openRejectHistoryLoading.value = false;
  }
}

async function openOpenRejectHistoryDialog() {
  openRejectHistoryVisible.value = true;
  await reloadOpenRejectHistory();
}

function focusPendingRequests() {
  openRequestStatus.value = "pending";
  void reloadOpenRequests();
}

function applyOpenRequestToProvision(row: UserRequest) {
  if (String(row.request_type || "").trim() !== "open") {
    ElMessage.info("仅“开通申请”支持带入开通表单");
    return;
  }
  const billing = String(row.billing_username || "").trim();
  if (!billing) return;
  provisionForm.billing_username = billing;
  const node = String(row.node_id || "").trim();
  const local = String(row.local_username || "").trim();
  if (node && node !== "待分配") {
    provisionForm.node_id = node;
    applyProvisionNodeDefaults(node);
  }
  if (local && local !== "待分配") {
    provisionForm.local_username = local;
  }
  syncProvisionUserPreviewFromLocal();
  void fetchProvisionUserDetail(true);
  provisionActionError.value = "";
  provisionActionSuccess.value = `已将申请 ${row.request_id} 带入开通表单，请核对后执行开通`;
  ElMessage.success("已带入开通表单，请在当前区域核对后执行开通");
}

async function markOpenRequestProcessed(row: UserRequest) {
  const requestID = Number(row.request_id || 0);
  if (!requestID || String(row.status || "").trim() !== "pending") return;
  try {
    await ElMessageBox.confirm(
      `确认将申请 ${requestID} 标记为“已处理”吗？\n平台账号：${row.billing_username}\n\n建议在节点账号开通完成后再标记。`,
      "标记已处理",
      { type: "warning", confirmButtonText: "确认标记", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  openRequestActionLoadingId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    await client().adminApproveRequest(requestID);
    success.value = `申请 ${requestID} 已标记为已处理`;
    await reloadOpenRequests();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequestActionLoadingId.value = 0;
  }
}

async function markOpenRequestPending(row: UserRequest) {
  const requestID = Number(row.request_id || 0);
  if (!requestID || String(row.status || "").trim() === "pending") return;
  try {
    await ElMessageBox.confirm(
      `确认将申请 ${requestID} 恢复为“待处理”吗？\n平台账号：${row.billing_username}\n当前状态：${requestStatusText(row.status)}\n\n适用于误点“标记已处理”的情况。`,
      "恢复待处理",
      { type: "warning", confirmButtonText: "确认恢复", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  openRequestActionLoadingId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    await client().adminReopenRequest(requestID);
    success.value = `申请 ${requestID} 已恢复为待处理`;
    await reloadOpenRequests();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequestActionLoadingId.value = 0;
  }
}

async function markOpenRequestRejected(row: UserRequest) {
  const requestID = Number(row.request_id || 0);
  if (!requestID || String(row.status || "").trim() !== "pending") return;
  let reason = "";
  try {
    const input: any = await ElMessageBox.prompt(
      `请填写拒绝理由（必填，用户端可见）：\n平台账号：${row.billing_username}\n申请类型：${requestTypeText(row.request_type)}\n节点：${row.node_id || "-"}\n节点账号：${row.local_username || "-"}`,
      "拒绝申请",
      {
        type: "warning",
        confirmButtonText: "确认拒绝",
        cancelButtonText: "取消",
        inputType: "textarea",
        inputPlaceholder: "例如：申请信息不完整，请补充后重提",
        inputValidator: (v: string) => String(v || "").trim().length > 0 || "拒绝理由不能为空",
      },
    );
    reason = String(input?.value || "").trim();
  } catch {
    return;
  }
  openRequestActionLoadingId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    await client().adminRejectRequest(requestID, reason);
    success.value = `申请 ${requestID} 已拒绝`;
    await reloadOpenRequests();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequestActionLoadingId.value = 0;
  }
}

async function copyText(text: string) {
  const value = String(text || "").trim();
  if (!value) return;
  try {
    await writeClipboardText(value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.error("复制失败，请手动复制");
  }
}

async function provisionAccount() {
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  const billing = provisionForm.billing_username.trim();
  const node = provisionForm.node_id.trim();
  const local = provisionForm.local_username.trim();
  if (!billing || !node || !local) {
    provisionActionError.value = "平台账号、节点编号、节点账号不能为空";
    await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  await fetchProvisionUserDetail(false);
  if (!provisionUserDetail.value || String(provisionUserDetail.value.username || "").trim() !== billing) {
    provisionActionError.value = provisionUserError.value || "请先确认平台账号详细信息，确认无误后再开通";
    await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  if (!/^[a-z_][a-z0-9_-]{0,31}$/.test(local)) {
    provisionActionError.value = "节点账号格式不合法：需以小写字母或下划线开头，只能包含小写字母、数字、下划线、短横线";
    await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  const sshHost = provisionForm.ssh_host.trim();
  const sshPort = Number(provisionForm.ssh_port || 0);
  rememberProvisionSSHHost(sshHost);
  try {
    await ElMessageBox.confirm(
      `请确认本次开通绑定信息：\n平台账号：${billing}\n节点编号：${node}\n节点账号：${local}\n\n确认后将生成密钥并发送邮件，绑定错误会影响用户登录。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认开通并绑定", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  const applyProvisionSuccess = async (r: AdminAccountProvisionResp, forceReissued = false) => {
    provisionResult.value = r;
    provisionResultVisible.value = true;
    const isReissued = !!r.reissued_key || forceReissued;
    const reusedExistingLocalUser = !!r.local_user_existed && !isReissued;
    if (r.mail_sent) {
      provisionActionSuccess.value = isReissued
        ? `新密钥已重新生成；密文已发平台内通知，提取码已发送到 ${r.email}`
        : reusedExistingLocalUser
          ? `已复用节点上的现有账号并刷新密钥；密文已发平台内通知，提取码已发送到 ${r.email}`
        : `节点账号已开通；密文已发平台内通知，提取码已发送到 ${r.email}`;
    } else {
      provisionActionSuccess.value = isReissued
        ? "新密钥已重新生成，平台内密文通知已生成，但提取码邮件发送失败"
        : reusedExistingLocalUser
          ? "已复用节点上的现有账号并刷新密钥，平台内密文通知已生成，但提取码邮件发送失败"
        : "节点账号已开通，平台内密文通知已生成，但提取码邮件发送失败";
    }
    await reloadAll();
  };
  const parseProvisionConflict = (e: any): { shouldOfferRotate: boolean; shouldConfirmExistingLocalUser: boolean; mappedBilling: string } => {
    const status = Number(e?.status || 0);
    if (status !== 409) return { shouldOfferRotate: false, shouldConfirmExistingLocalUser: false, mappedBilling: "" };
    const msg = String(e?.message || "").trim();
    let reason = "";
    let mappedBilling = "";
    const bodyText = String(e?.body || "").trim();
    if (bodyText) {
      try {
        const j = JSON.parse(bodyText);
        reason = String(j?.reason || "").trim();
        mappedBilling = String(j?.mapped_billing_username || "").trim();
      } catch {
        // ignore parse failure
      }
    }
    if (reason === "local_user_exists_unmapped") {
      return { shouldOfferRotate: false, shouldConfirmExistingLocalUser: true, mappedBilling };
    }
    if (reason === "mapping_exists_other_user") {
      return { shouldOfferRotate: false, shouldConfirmExistingLocalUser: false, mappedBilling };
    }
    const sameUserReason = reason === "mapping_exists_same_user";
    const compatibleLegacyReason = reason === "mapping_exists" && (!mappedBilling || mappedBilling === billing);
    const legacyMsgHint = !reason && (msg.includes("已有平台映射") || msg.includes("已绑定到平台账号"));
    return {
      shouldOfferRotate: sameUserReason || compatibleLegacyReason || legacyMsgHint,
      shouldConfirmExistingLocalUser: false,
      mappedBilling,
    };
  };

  provisioning.value = true;
  try {
    const r = await client().adminProvisionAccount({
      billing_username: billing,
      node_id: node,
      local_username: local,
      ssh_host: sshHost || undefined,
      ssh_port: sshPort > 0 ? sshPort : undefined,
    });
    await applyProvisionSuccess(r, false);
  } catch (e: any) {
    const conflict = parseProvisionConflict(e);
    if (conflict.shouldConfirmExistingLocalUser) {
      try {
        await ElMessageBox.confirm(
          `高风险提醒：节点上已经存在这个本地账号，但平台当前没有该账号映射。\n\n节点编号：${node}\n节点账号：${local}\n平台账号：${billing}\n\n继续后系统会复用节点上的现有账号，并把该账号的 authorized_keys 覆盖为新生成的公钥；旧私钥可能无法继续登录。请确认你已经核对过该账号归属。`,
          "高风险确认：复用现有节点账号",
          { type: "error", confirmButtonText: "确认复用并覆盖 authorized_keys", cancelButtonText: "取消" },
        );
      } catch {
        return;
      }
      try {
        const r2 = await client().adminProvisionAccount({
          billing_username: billing,
          node_id: node,
          local_username: local,
          ssh_host: sshHost || undefined,
          ssh_port: sshPort > 0 ? sshPort : undefined,
          confirm_existing_local_user: true,
        });
        await applyProvisionSuccess(r2, false);
      } catch (e2: any) {
        provisionActionError.value = e2?.message ?? String(e2);
        await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
      }
      return;
    }
    if (!conflict.shouldOfferRotate) {
      provisionActionError.value = e?.message ?? String(e);
      await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
      return;
    }
    const mapped = conflict.mappedBilling || billing;
    try {
      await ElMessageBox.confirm(
        `检测到该节点账号已存在映射。\n节点编号：${node}\n节点账号：${local}\n平台账号：${mapped}\n\n该用户可能是丢失了旧密钥。是否立即重新生成新密钥，并重新发送“平台内密文 + 邮件提取码”？`,
        "二次提醒：是否重发新密钥",
        { type: "warning", confirmButtonText: "重新生成并重发", cancelButtonText: "取消" },
      );
    } catch {
      return;
    }
    try {
      const r2 = await client().adminProvisionAccount({
        billing_username: billing,
        node_id: node,
        local_username: local,
        ssh_host: sshHost || undefined,
        ssh_port: sshPort > 0 ? sshPort : undefined,
        rotate_key: true,
      });
      await applyProvisionSuccess(r2, true);
    } catch (e2: any) {
      provisionActionError.value = e2?.message ?? String(e2);
      await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
    }
  } finally {
    provisioning.value = false;
  }
}

async function reloadAll() {
  pageLoading.value = true;
  error.value = "";
  try {
    await Promise.all([
      loadPlatformUsers(),
      loadNodeOptions(),
      reloadProvisionLogs(),
      reloadOpenRequests(),
    ]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    pageLoading.value = false;
  }
}

reloadAll();
</script>

<style scoped>
.admin-account-provision-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.head,
.head-actions,
.section-title-wrap,
.preview-head,
.provision-actions,
.pending-open-actions,
.kv-row {
  display: flex;
  align-items: center;
}

.head,
.preview-head {
  justify-content: space-between;
  gap: 12px;
}

.head-actions,
.pending-open-actions,
.provision-actions {
  gap: 8px;
  flex-wrap: wrap;
}

.section-title-wrap {
  gap: 10px;
  font-weight: 600;
}

.provision-history-head {
  justify-content: flex-start;
}

.section-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
}

.tone-provision {
  color: var(--el-color-success);
}

.tone-note {
  color: var(--el-color-warning);
}

.tone-history {
  color: var(--el-color-primary);
}

.tone-user {
  color: var(--el-color-info);
}

.tone-node {
  color: var(--el-color-primary);
}

.mb {
  margin-bottom: 12px;
}

.mini {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.preview-desc,
.provision-node-preview,
.provision-user-preview,
.pending-open-banner,
.note-user-detail {
  margin-bottom: 12px;
}

.node-option-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.35;
}

.node-option-title {
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.node-option-desc {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.note-user-lines {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.kv-row {
  gap: 12px;
  margin-top: 12px;
}

.kv-label {
  width: 120px;
  flex: 0 0 120px;
  color: var(--el-text-color-secondary);
}

.payload-actions {
  margin-top: 8px;
}

@media (max-width: 900px) {
  .kv-row {
    flex-direction: column;
    align-items: stretch;
  }

  .kv-label {
    width: auto;
    flex: 1 1 auto;
  }
}
</style>
