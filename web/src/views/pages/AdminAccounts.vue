<template>
  <div class="admin-accounts-page">
    <el-card class="section-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-map"><el-icon><Connection /></el-icon></span>
            <span>平台账号映射管理（管理员）</span>
          </div>
          <div class="head-actions">
            <el-button :loading="loading" @click="reload">刷新</el-button>
            <el-button type="primary" @click="openCreateDialog">新增映射</el-button>
          </div>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
      <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
      <el-alert title="同一平台账号可映射多个节点账号；唯一键是“节点编号 + 节点账号”。" type="info" show-icon class="mb" />
    </el-card>

    <el-card class="section-card risk-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-risk"><el-icon><WarningFilled /></el-icon></span>
            <span>异常换绑监测</span>
            <el-badge :value="mappingRisks.length" :hidden="mappingRisks.length <= 0" type="danger" class="pending-count-badge" />
          </div>
          <div class="head-actions">
            <el-input-number v-model="riskDays" :min="1" :max="365" :step="1" size="small" style="width: 130px" />
            <el-input-number v-model="riskMinSwitches" :min="1" :max="20" :step="1" size="small" style="width: 130px" />
            <el-button size="small" :loading="mappingRiskLoading" @click="reloadMappingRisks">刷新监测</el-button>
          </div>
        </div>
      </template>
      <el-alert
        title="检测规则：同一“节点编号+节点账号”在监测窗口内换绑次数较多，或涉及多个平台账号时标记为风险。"
        type="warning"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-table :data="mappingRisks" stripe size="small" max-height="260" empty-text="暂无异常换绑账号">
        <el-table-column label="风险" width="70">
          <template #default>
            <span class="risk-dot" />
          </template>
        </el-table-column>
        <el-table-column prop="node_id" label="节点编号" width="120" />
        <el-table-column prop="local_username" label="节点账号" width="150" />
        <el-table-column prop="current_billing_username" label="当前平台账号" width="170" />
        <el-table-column label="风险指标" width="240">
          <template #default="{ row }">
            <el-tag type="danger" size="small">换绑 {{ Number(row.switch_count || 0) }} 次</el-tag>
            <el-tag type="warning" size="small" style="margin-left: 6px">涉及 {{ Number(row.distinct_billing_count || 0) }} 人</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="换绑轨迹" min-width="320">
          <template #default="{ row }">
            <div v-if="(row.switch_history || []).length" class="switch-history-wrap">
              <div v-for="(item, idx) in row.switch_history || []" :key="`${row.node_id}:${row.local_username}:sw:${idx}`" class="switch-history-item">
                <span>{{ formatSwitchHistoryItem(item) }}</span>
              </div>
            </div>
            <span v-else class="mini">暂无明确换绑链路</span>
          </template>
        </el-table-column>
        <el-table-column label="涉及平台账号（可点击详情/拉黑）" min-width="380">
          <template #default="{ row }">
            <div class="risk-users">
              <div v-for="u in row.platform_usernames || []" :key="`${row.node_id}:${row.local_username}:${u}`" class="risk-user-item">
                <el-button link type="primary" @click="openProfile(u)">{{ u }}</el-button>
                <el-tag v-if="isPlatformBlocked(u)" type="danger" size="small">已拉黑</el-tag>
                <el-button
                  size="small"
                  :type="isPlatformBlocked(u) ? 'warning' : 'danger'"
                  plain
                  @click="toggleRiskUserBlock(u, row)"
                >
                  {{ isPlatformBlocked(u) ? "解黑" : "拉黑" }}
                </el-button>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="最近变更时间" min-width="170">
          <template #default="{ row }">{{ fmtTime(row.last_changed_at) }}</template>
        </el-table-column>
      </el-table>
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
                @select="onProvisionNodeSelect"
              />
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
      <div class="provision-actions">
        <el-button type="success" :loading="provisioning" @click="provisionAccount">开通账号并发送提取码邮件</el-button>
      </div>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-note"><el-icon><Document /></el-icon></span>
            <span>开通处理记事（用户申请）</span>
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
            <el-button size="small" :loading="openRequestsLoading" @click="reloadOpenRequests">刷新</el-button>
          </div>
        </div>
      </template>
      <el-alert
        title="这里集中展示“用户申请开通节点账号”的记事。你可以先“带入开通表单”完成开通，再点击“标记已处理”。"
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
      <el-table :data="openRequests" stripe size="small" max-height="360">
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
        <el-table-column prop="message" label="申请理由（含研究方向）" min-width="300" />
        <el-table-column label="申请时间" min-width="170">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="320" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button size="small" @click="applyOpenRequestToProvision(row)">带入开通</el-button>
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

    <el-card class="section-card">
      <template #header>
        <div class="head section-title-wrap">
          <span class="section-icon tone-history"><el-icon><Clock /></el-icon></span>
          <span>节点账号开通历史</span>
          <el-button size="small" :loading="provisionLogsLoading" @click="reloadProvisionLogs">刷新历史</el-button>
        </div>
      </template>
      <el-table :data="provisionLogs" stripe size="small" max-height="280">
        <el-table-column label="开通时间" min-width="172">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="created_by" label="操作人" width="120" />
        <el-table-column prop="billing_username" label="平台账号" width="160" />
        <el-table-column prop="node_id" label="节点编号" width="120" />
        <el-table-column prop="local_username" label="节点账号" width="150" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column label="邮件状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.mail_sent ? 'success' : 'danger'" size="small">{{ row.mail_sent ? "成功" : "失败" }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="mail_error" label="失败原因" min-width="220" />
      </el-table>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-query"><el-icon><Search /></el-icon></span>
          <span>映射查询</span>
        </div>
      </template>
      <el-form inline class="query-form">
        <el-form-item label="平台账号查询">
          <el-autocomplete
            v-model="filterBilling"
            style="width: 260px"
            clearable
            placeholder="输入平台账号筛选"
            :fetch-suggestions="queryBillingOptions"
            @select="onFilterBillingSelect"
          />
        </el-form-item>
        <el-form-item><el-button @click="reload">查询</el-button></el-form-item>
        <el-form-item><el-button @click="resetFilter">重置</el-button></el-form-item>
      </el-form>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-list"><el-icon><List /></el-icon></span>
            <span>映射列表</span>
          </div>
          <el-tag type="info" effect="plain">共 {{ rows.length }} 条</el-tag>
        </div>
      </template>
      <el-table :data="rows" stripe>
        <el-table-column label="平台账号" width="190">
          <template #default="{ row }">
            <div class="map-user-cell">
              <span v-if="isRiskRow(row)" class="risk-dot mini" />
              <el-button link type="primary" @click="openProfile(row.billing_username)">{{ row.billing_username }}</el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="node_id" label="节点编号" width="140" />
        <el-table-column prop="local_username" label="节点账号" width="190" />
        <el-table-column label="更新时间" min-width="180">
          <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="220">
          <template #default="{ row }">
            <el-button size="small" @click="openEditDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="editVisible"
      :title="editMode === 'create' ? '新增映射' : '编辑映射'"
      width="620px"
      destroy-on-close
    >
      <el-form label-width="100px">
        <el-form-item label="平台账号">
          <el-autocomplete
            v-model="formBilling"
            style="width: 100%"
            clearable
            placeholder="输入平台账号"
            :fetch-suggestions="queryBillingOptions"
            @select="onFormBillingSelect"
          />
        </el-form-item>
        <el-form-item label="节点编号">
          <el-autocomplete
            v-model="nodeId"
            style="width: 100%"
            clearable
            placeholder="输入节点编号，例如 60000"
            :fetch-suggestions="queryNodeOptions"
            @select="onNodeSelect"
          />
        </el-form-item>
        <el-form-item label="节点账号">
          <el-autocomplete
            v-model="localUsername"
            style="width: 100%"
            clearable
            placeholder="输入节点账号"
            :fetch-suggestions="queryLocalUserOptions"
            @select="onLocalUserSelect"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">
          {{ editMode === "create" ? "新增" : "保存修改" }}
        </el-button>
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
  type PlatformUserDetail,
  type SSHBlacklistEntry,
  type UserRequest,
  type UserNodeAccount,
  type UserNodeAccountMappingRisk,
} from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime } from "../../lib/time";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { Clock, Connection, Document, Key, List, Search, UserFilled, WarningFilled } from "@element-plus/icons-vue";

const loading = ref(false);
const saving = ref(false);
const provisioning = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<UserNodeAccount[]>([]);
const mappingRiskLoading = ref(false);
const mappingRisks = ref<UserNodeAccountMappingRisk[]>([]);
const riskDays = ref(30);
const riskMinSwitches = ref(2);
const filterBilling = ref("");
const formBilling = ref("");
const nodeId = ref("");
const localUsername = ref("");
const billingOptions = ref<string[]>([]);
const localUserOptions = ref<string[]>([]);
const nodeOptions = ref<string[]>([]);
const nodeIPByID = ref<Record<string, string>>({});
const platformUsers = ref<AdminUserDetail[]>([]);
const profileVisible = ref(false);
const selectedProfileUsername = ref("");
const editVisible = ref(false);
const editMode = ref<"create" | "edit">("create");
const old = ref<{ billing: string; node: string; local: string } | null>(null);

const provisionResultVisible = ref(false);
const provisionResult = ref<AdminAccountProvisionResp | null>(null);
const provisionLogsLoading = ref(false);
const provisionLogs = ref<AdminAccountProvisionLog[]>([]);
const openRequestsLoading = ref(false);
const openRequests = ref<UserRequest[]>([]);
const openRequestStatus = ref<"pending" | "approved" | "rejected" | "">("pending");
const openRequestActionLoadingId = ref(0);
const pendingOpenRequests = ref<UserRequest[]>([]);
const blockedIdentities = ref<Set<string>>(new Set());
const provisionUserLoading = ref(false);
const provisionUserError = ref("");
const provisionUserDetail = ref<PlatformUserDetail | null>(null);
const provisionUserLastFetched = ref("");
let provisionUserFetchSeq = 0;
const DEFAULT_PROVISION_SSH_HOST = "controller.example.org";
const PROVISION_SSH_HOST_STORAGE_KEY = "gpuops.provision.last_ssh_host";

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

const blockedPlatformUserSet = computed(() => {
  const set = new Set<string>();
  for (const x of blockedIdentities.value) {
    const v = String(x || "").trim();
    if (v) set.add(v);
  }
  for (const u of platformUsers.value || []) {
    const username = String(u.username || "").trim();
    if (!username) continue;
    if (String(u.status || "").trim() === "blocked") {
      set.add(username);
    }
  }
  return set;
});

function uniqSorted(items: string[]): string[] {
  const s = new Set<string>();
  for (const item of items) {
    const v = String(item || "").trim();
    if (v) s.add(v);
  }
  return Array.from(s).sort((a, b) => a.localeCompare(b));
}

function mappingKey(nodeID: string, localUser: string): string {
  return `${String(nodeID || "").trim()}::${String(localUser || "").trim()}`;
}

const riskKeySet = computed(() => {
  const set = new Set<string>();
  for (const r of mappingRisks.value || []) {
    const key = mappingKey(r.node_id, r.local_username);
    if (key) set.add(key);
  }
  return set;
});

function isRiskRow(row: UserNodeAccount): boolean {
  return riskKeySet.value.has(mappingKey(row.node_id, row.local_username));
}

function isPlatformBlocked(username: string): boolean {
  const u = String(username || "").trim();
  if (!u) return false;
  return blockedPlatformUserSet.value.has(u);
}

function formatSwitchHistoryItem(item: string): string {
  const s = String(item || "").trim();
  if (!s) return "";
  return s.replace(/\s*->\s*/g, " \u2192 ");
}

function fmtTime(v: string): string {
  return formatServerDateTime(v);
}

function collectBlockedPlatformUser(set: Set<string>, value: string) {
  const v = String(value || "").trim();
  if (!v) return;
  set.add(v);
}

function buildBlockedIdentitySet(entries: SSHBlacklistEntry[]): Set<string> {
  const set = new Set<string>();
  for (const e of entries || []) {
    const sourcePlatform = String(e.source_platform_username || "").trim();
    const billing = String(e.billing_username || "").trim();
    if (sourcePlatform) {
      collectBlockedPlatformUser(set, sourcePlatform);
      continue;
    }
    if (billing) {
      collectBlockedPlatformUser(set, billing);
    }
  }
  return set;
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

function queryNodeOptions(queryString: string, cb: (items: Array<{ value: string }>) => void) {
  queryOptions(nodeOptions.value, queryString, cb);
}

function queryLocalUserOptions(queryString: string, cb: (items: Array<{ value: string }>) => void) {
  queryOptions(localUserOptions.value, queryString, cb);
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

function onFilterBillingSelect(item: { value?: string }) {
  filterBilling.value = String(item?.value || "").trim();
}

function onFormBillingSelect(item: { value?: string }) {
  formBilling.value = String(item?.value || "").trim();
}

function onNodeSelect(item: { value?: string }) {
  nodeId.value = String(item?.value || "").trim();
}

function onLocalUserSelect(item: { value?: string }) {
  localUsername.value = String(item?.value || "").trim();
}

function onProvisionBillingSelect(item: { value?: string }) {
  provisionForm.billing_username = String(item?.value || "").trim();
  syncProvisionUserPreviewFromLocal();
  void fetchProvisionUserDetail(true);
}

function onProvisionBillingChange() {
  syncProvisionUserPreviewFromLocal();
}

function onProvisionBillingBlur() {
  void fetchProvisionUserDetail(false);
}

function onProvisionNodeSelect(item: { value?: string }) {
  provisionForm.node_id = String(item?.value || "").trim();
  applyProvisionNodeDefaults(provisionForm.node_id);
}

function openProfile(username: string) {
  selectedProfileUsername.value = String(username || "").trim();
  if (!selectedProfileUsername.value) return;
  profileVisible.value = true;
}

function fmt2(v: number): string {
  return Number(v || 0).toFixed(2);
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
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminPlatformUserDetail(username);
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

function resetFilter() {
  filterBilling.value = "";
  reload();
}

function clearEditForm() {
  formBilling.value = "";
  nodeId.value = "";
  localUsername.value = "";
  old.value = null;
}

function openCreateDialog() {
  clearEditForm();
  editMode.value = "create";
  editVisible.value = true;
}

function openEditDialog(row: UserNodeAccount) {
  editMode.value = "edit";
  old.value = { billing: row.billing_username, node: row.node_id, local: row.local_username };
  formBilling.value = row.billing_username;
  nodeId.value = row.node_id;
  localUsername.value = row.local_username;
  editVisible.value = true;
}

function mergeBillingOptions(accounts: UserNodeAccount[], users: AdminUserDetail[]) {
  const values: string[] = [];
  for (const a of accounts) values.push(a.billing_username || "");
  for (const u of users) values.push(u.username || "");
  billingOptions.value = uniqSorted(values);
}

function refreshLocalOptions(accounts: UserNodeAccount[]) {
  localUserOptions.value = uniqSorted(accounts.map((x) => x.local_username || ""));
}

async function loadNodeOptions() {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const r = await client.adminNodes(3000);
  const ids: string[] = [];
  const ipMap: Record<string, string> = {};
  for (const n of r.nodes ?? []) {
    const id = String(n.node_id || "").trim();
    if (!id) continue;
    ids.push(id);
    ipMap[id] = String(n.node_ip || "").trim();
  }
  nodeOptions.value = uniqSorted(ids);
  nodeIPByID.value = ipMap;
}

async function loadPlatformUsers() {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const r = await client.adminUsersDetails(3000);
  platformUsers.value = r.users ?? [];
  syncProvisionUserPreviewFromLocal();
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminAccounts(filterBilling.value.trim());
    rows.value = r.accounts ?? [];
    refreshLocalOptions(rows.value);
    mergeBillingOptions(rows.value, platformUsers.value);
    await Promise.all([reloadProvisionLogs(), reloadOpenRequests(), reloadMappingRisks(), reloadBlockedIdentities()]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function reloadMappingRisks() {
  mappingRiskLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminAccountMappingRisks({
      days: Number(riskDays.value || 30),
      min_switches: Number(riskMinSwitches.value || 2),
      limit: 500,
    });
    mappingRisks.value = r.risky_accounts ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    mappingRiskLoading.value = false;
  }
}

async function reloadBlockedIdentities() {
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminBlacklist("");
    blockedIdentities.value = buildBlockedIdentitySet(r.entries ?? []);
  } catch {
    blockedIdentities.value = new Set<string>();
  }
}

async function toggleRiskUserBlock(username: string, row: UserNodeAccountMappingRisk) {
  const u = String(username || "").trim();
  if (!u) return;
  const blocked = isPlatformBlocked(u);
  const title = blocked ? "解黑确认" : "拉黑确认";
  const body = blocked
    ? `确认解除平台账号 ${u} 的黑名单吗？\n节点：${row.node_id}\n节点账号：${row.local_username}\n\n解除后该账号可重新按映射规则登录。`
    : `确认拉黑平台账号 ${u} 吗？\n节点：${row.node_id}\n节点账号：${row.local_username}\n\n拉黑后会覆盖其所有节点映射，立即断开 SSH 并清理进程。`;
  try {
    await ElMessageBox.confirm(
      body,
      title,
      {
        type: "warning",
        confirmButtonText: blocked ? "确认解黑" : "确认拉黑",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (blocked) {
      await client.adminUnblockUser(u);
      success.value = `已解除平台账号 ${u} 黑名单`;
    } else {
      await client.adminBlockUser(u, `换绑风险处置：节点 ${row.node_id} 账号 ${row.local_username}`);
      success.value = `已拉黑平台账号 ${u}，并已对其全部映射节点生效`;
    }
    await loadPlatformUsers();
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function reloadProvisionLogs() {
  provisionLogsLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminProvisionLogs({
      billing_username: filterBilling.value.trim() || undefined,
      limit: 200,
    });
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
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminRequests({ status: openRequestStatus.value || "", limit: 5000 });
    const current = (r.requests ?? []).filter((x) => String(x.request_type || "").trim() === "open");
    openRequests.value = current;
    if (!openRequestStatus.value || openRequestStatus.value === "pending") {
      pendingOpenRequests.value = current.filter((x) => String(x.status || "").trim() === "pending");
      return;
    }
    const pendingResp = await client.adminRequests({ status: "pending", limit: 5000 });
    pendingOpenRequests.value = (pendingResp.requests ?? []).filter((x) => String(x.request_type || "").trim() === "open");
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequestsLoading.value = false;
  }
}

function focusPendingRequests() {
  openRequestStatus.value = "pending";
  void reloadOpenRequests();
}

function applyOpenRequestToProvision(row: UserRequest) {
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
  success.value = `已将申请 ${row.request_id} 带入开通表单，请核对后执行开通`;
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
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminApproveRequest(requestID);
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
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminReopenRequest(requestID);
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
  try {
    await ElMessageBox.confirm(
      `确认拒绝申请 ${requestID} 吗？\n平台账号：${row.billing_username}\n\n拒绝后用户可重新提交新的开通申请。`,
      "拒绝申请",
      { type: "warning", confirmButtonText: "确认拒绝", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  openRequestActionLoadingId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminRejectRequest(requestID);
    success.value = `申请 ${requestID} 已拒绝`;
    await reloadOpenRequests();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequestActionLoadingId.value = 0;
  }
}

async function save() {
  error.value = "";
  success.value = "";
  const billing = formBilling.value.trim();
  const node = nodeId.value.trim();
  const local = localUsername.value.trim();
  if (!billing || !node || !local) {
    error.value = "平台账号、节点编号、节点账号均不能为空";
    return;
  }
  saving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (editMode.value === "edit" && old.value) {
      await client.adminUpdateAccount({
        old_billing_username: old.value.billing,
        old_node_id: old.value.node,
        old_local_username: old.value.local,
        new_billing_username: billing,
        new_node_id: node,
        new_local_username: local,
      });
      success.value = "修改成功";
    } else {
      await client.adminUpsertAccount({
        billing_username: billing,
        node_id: node,
        local_username: local,
      });
      success.value = "新增成功";
    }
    editVisible.value = false;
    clearEditForm();
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    saving.value = false;
  }
}

async function remove(row: UserNodeAccount) {
  error.value = "";
  success.value = "";
  try {
    await ElMessageBox.confirm(
      `确认删除映射吗？\n平台账号：${row.billing_username}\n节点：${row.node_id}\n节点账号：${row.local_username}`,
      "删除确认",
      { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminDeleteAccount({
      billing_username: row.billing_username,
      node_id: row.node_id,
      local_username: row.local_username,
    });
    success.value = "删除成功";
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function copyText(text: string) {
  const value = String(text || "").trim();
  if (!value) return;
  try {
    await navigator.clipboard.writeText(value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.error("复制失败，请手动复制");
  }
}

async function provisionAccount() {
  error.value = "";
  success.value = "";
  const billing = provisionForm.billing_username.trim();
  const node = provisionForm.node_id.trim();
  const local = provisionForm.local_username.trim();
  if (!billing || !node || !local) {
    error.value = "平台账号、节点编号、节点账号不能为空";
    return;
  }
  await fetchProvisionUserDetail(false);
  if (!provisionUserDetail.value || String(provisionUserDetail.value.username || "").trim() !== billing) {
    error.value = provisionUserError.value || "请先确认平台账号详细信息，确认无误后再开通";
    return;
  }
  if (!/^[a-z_][a-z0-9_-]{0,31}$/.test(local)) {
    error.value = "节点账号格式不合法：需以小写字母或下划线开头，只能包含小写字母、数字、下划线、短横线";
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
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const applyProvisionSuccess = async (r: AdminAccountProvisionResp, forceReissued = false) => {
    provisionResult.value = r;
    provisionResultVisible.value = true;
    const isReissued = !!r.reissued_key || forceReissued;
    if (r.mail_sent) {
      success.value = isReissued
        ? `新密钥已重新生成；密文已发平台内通知，提取码已发送到 ${r.email}`
        : `节点账号已开通；密文已发平台内通知，提取码已发送到 ${r.email}`;
    } else {
      success.value = isReissued
        ? "新密钥已重新生成，平台内密文通知已生成，但提取码邮件发送失败"
        : "节点账号已开通，平台内密文通知已生成，但提取码邮件发送失败";
    }
    await reload();
    await reloadProvisionLogs();
  };

  const parseProvisionConflict = (e: any): { shouldOfferRotate: boolean; mappedBilling: string } => {
    const status = Number(e?.status || 0);
    if (status !== 409) return { shouldOfferRotate: false, mappedBilling: "" };
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
        // ignore parse failure and fallback to message matching
      }
    }
    if (reason === "mapping_exists_other_user") {
      return { shouldOfferRotate: false, mappedBilling };
    }
    const sameUserReason = reason === "mapping_exists_same_user";
    const compatibleLegacyReason = reason === "mapping_exists" && (!mappedBilling || mappedBilling === billing);
    const legacyMsgHint = !reason && (msg.includes("已有平台映射") || msg.includes("已绑定到平台账号"));
    return {
      shouldOfferRotate: sameUserReason || compatibleLegacyReason || legacyMsgHint,
      mappedBilling,
    };
  };

  provisioning.value = true;
  try {
    const r = await client.adminProvisionAccount({
      billing_username: billing,
      node_id: node,
      local_username: local,
      ssh_host: sshHost || undefined,
      ssh_port: sshPort > 0 ? sshPort : undefined,
    });
    await applyProvisionSuccess(r, false);
  } catch (e: any) {
    const conflict = parseProvisionConflict(e);
    if (!conflict.shouldOfferRotate) {
      error.value = e?.message ?? String(e);
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
      const r2 = await client.adminProvisionAccount({
        billing_username: billing,
        node_id: node,
        local_username: local,
        ssh_host: sshHost || undefined,
        ssh_port: sshPort > 0 ? sshPort : undefined,
        rotate_key: true,
      });
      await applyProvisionSuccess(r2, true);
    } catch (e2: any) {
      error.value = e2?.message ?? String(e2);
    }
  } finally {
    provisioning.value = false;
  }
}

async function init() {
  try {
    await loadPlatformUsers();
  } catch {
    platformUsers.value = [];
  }
  try {
    await loadNodeOptions();
  } catch {
    nodeOptions.value = [];
    nodeIPByID.value = {};
  }
  await reload();
}

init();
</script>

<style scoped>
.admin-accounts-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-card {
  border: 1px solid var(--border-color);
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.05);
}

.section-card :deep(.el-card__header) {
  padding: 12px 16px;
  background: #f7fbff;
  border-bottom: 1px solid var(--border-color);
}

.head { display: flex; justify-content: space-between; align-items: center; gap: 10px; }
.head-actions { display: flex; gap: 8px; }
.section-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-weight: 700;
  color: var(--text-primary);
}
.section-icon {
  width: 30px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.35),
    0 4px 12px rgba(15, 23, 42, 0.15);
}
.section-icon :deep(svg) {
  width: 17px;
  height: 17px;
}
.tone-map {
  background: linear-gradient(135deg, #0f766e, #0d9488);
  color: #ccfbf1;
}
.tone-provision {
  background: linear-gradient(135deg, #4f46e5, #4338ca);
  color: #e0e7ff;
}
.tone-history {
  background: linear-gradient(135deg, #b45309, #d97706);
  color: #fef3c7;
}
.tone-query {
  background: linear-gradient(135deg, #0369a1, #0284c7);
  color: #e0f2fe;
}
.tone-list {
  background: linear-gradient(135deg, #be123c, #e11d48);
  color: #ffe4e6;
}
.tone-note {
  background: linear-gradient(135deg, #0369a1, #0ea5e9);
  color: #e0f2fe;
}
.tone-risk {
  background: linear-gradient(135deg, #b91c1c, #ef4444);
  color: #fee2e2;
}
.tone-user {
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
  color: #dbeafe;
}
.mb { margin-bottom: 12px; }
.risk-card :deep(.el-card__body) {
  padding-top: 12px;
}
.risk-users {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.switch-history-wrap {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.switch-history-item {
  display: inline-flex;
  align-items: center;
  color: #991b1b;
  font-size: 12px;
  background: rgba(254, 226, 226, 0.65);
  border: 1px solid rgba(248, 113, 113, 0.3);
  border-radius: 7px;
  padding: 2px 8px;
}
.risk-user-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(248, 113, 113, 0.08);
  border: 1px solid rgba(248, 113, 113, 0.28);
  border-radius: 8px;
  padding: 2px 8px;
}
.risk-dot {
  display: inline-block;
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: #ef4444;
  box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.18);
}
.risk-dot.mini {
  width: 7px;
  height: 7px;
  margin-right: 6px;
}
.map-user-cell {
  display: inline-flex;
  align-items: center;
}
.provision-card :deep(.el-card__body) { padding-top: 14px; }
.provision-head { font-weight: 700; }
.provision-user-preview {
  margin: 8px 0 12px;
  padding: 10px 12px;
  border: 1px solid var(--border-color);
  border-radius: 10px;
  background: linear-gradient(180deg, rgba(148, 163, 184, 0.08), rgba(148, 163, 184, 0.02));
}
.preview-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.preview-desc :deep(.el-descriptions__label) {
  min-width: 90px;
}
.provision-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.query-form {
  margin-bottom: -2px;
}
.kv-row {
  margin-top: 12px;
  display: grid;
  grid-template-columns: 90px 1fr auto;
  gap: 8px;
  align-items: center;
}
.kv-label { color: #475569; font-size: 13px; }
.payload-actions {
  margin-top: 8px;
  display: flex;
  gap: 8px;
}
.note-user-detail {
  margin: 4px 0;
}
.note-user-lines {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.note-user-lines .line-1 {
  color: #0f172a;
  font-weight: 600;
}
.note-user-lines .line-2,
.note-user-lines .line-3 {
  color: #475569;
  font-size: 12px;
}
.pending-count-badge {
  margin-left: 2px;
}
.pending-count-badge :deep(.el-badge__content) {
  font-weight: 700;
}
.pending-open-banner {
  border: 1px solid #f59e0b;
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.12), rgba(245, 158, 11, 0.04));
}
.pending-open-actions {
  margin-top: 6px;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

@media (max-width: 900px) {
  .head {
    flex-wrap: wrap;
  }
}
</style>
