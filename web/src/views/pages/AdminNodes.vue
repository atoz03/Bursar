<template>
  <div class="page-container fade-in">
    <div class="page-header">
      <div class="header-content">
        <div class="header-icon">
          <el-icon :size="28"><Monitor /></el-icon>
        </div>
        <div>
          <h1 class="page-title">节点状态监控</h1>
          <p class="page-subtitle">实时监控集群节点运行状态和资源使用情况（节点在线状态基于 Agent 心跳，不等同于 SSH 连通性）</p>
        </div>
      </div>
      <div class="header-actions">
        <el-text type="info" size="small" class="refresh-time-text">上次刷新：{{ lastRefreshTimeText }}</el-text>
        <el-tooltip content="立即同步" placement="top">
          <el-button
            link
            class="icon-action-btn"
            :disabled="!detailNodeId || !canManageNodes"
            :loading="!!detailNodeId && syncingNodeId === detailNodeId"
            @click="syncCurrentNode"
          >
            <el-icon :size="20"><Refresh /></el-icon>
          </el-button>
        </el-tooltip>
        <el-button :loading="loading" type="primary" @click="reload" size="large" round>
          <el-icon><Refresh /></el-icon>
          刷新数据
        </el-button>
      </div>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon class="error-alert" />
    <el-alert
      v-if="riskyNodes.length > 0"
      type="warning"
      show-icon
      :closable="false"
      class="risk-banner"
    >
      <template #title>
        <span>🚨 安全提醒：近 7 天有 {{ riskyNodes.length }} 台节点出现安全事件或疑似恶意账号。</span>
      </template>
      <div class="risk-banner-list">
        <button
          v-for="node in riskyNodes"
          :key="`risk-${node.node_id}`"
          type="button"
          class="risk-chip"
          @click="openNodeDetailById(node.node_id)"
        >
          <span class="risk-chip-emoji">{{ node.emoji }}</span>
          <span>{{ node.node_id }}</span>
          <span class="risk-chip-meta">{{ node.summary }}</span>
        </button>
      </div>
    </el-alert>

    <!-- 统计卡片 -->
    <div class="stats-grid" v-if="rows.length > 0">
      <div class="stat-card">
        <div class="stat-icon" style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%)">
          <el-icon :size="24"><Monitor /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ onlineNodeCount }} / {{ rows.length }}</div>
          <div class="stat-label">在线节点 / 总节点</div>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon" style="background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%)">
          <el-icon :size="24"><Cpu /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ totalGpuProcesses }}</div>
          <div class="stat-label">GPU 进程总数</div>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon" style="background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%)">
          <el-icon :size="24"><Cpu /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ totalCpuProcesses }}</div>
          <div class="stat-label">CPU 进程总数</div>
        </div>
      </div>

      <div class="stat-card">
        <div class="stat-icon" style="background: linear-gradient(135deg, #fa709a 0%, #fee140 100%)">
          <el-icon :size="24"><Coin /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ totalCost.toFixed(2) }}</div>
          <div class="stat-label">总积分消耗</div>
        </div>
      </div>
    </div>

    <!-- 节点列表 -->
    <el-card class="table-card">
      <template #header>
        <div class="card-header">
          <div class="card-title">
            <el-icon><List /></el-icon>
            <span>节点详细信息</span>
          </div>
          <el-text type="info" size="small">共 {{ rows.length }} 个节点</el-text>
        </div>
      </template>

      <el-table
        :data="sortedRows"
        stripe
        style="width: 100%"
        :height="700"
        :header-cell-style="{ background: 'var(--bg-tertiary)', color: 'var(--text-primary)' }"
      >
        <el-table-column prop="node_id" label="节点ID" width="210" fixed>
          <template #default="{ row }">
            <div class="node-id-cell">
              <div class="node-id-head">
                <el-button link class="node-id-link" @click="openNodeDetail(row)">{{ row.node_id }}</el-button>
                <span
                  v-if="hasNodeSecurityRisk(row)"
                  class="risk-emoji"
                  :title="nodeRiskTooltip(row)"
                >
                  {{ nodeRiskEmoji(row) }}
                </span>
              </div>
              <div class="node-status-cluster">
                <el-tag
                  size="small"
                  effect="plain"
                  :type="nodeStatusTagType(row)"
                  class="node-status-tag"
                  :class="{ 'node-status-tag-clickable': canClickNodeHeartbeatTag(row) }"
                  @click="onNodeHeartbeatTagClick(row)"
                >
                  {{ nodeStatusText(row) }}
                </el-tag>
                <el-tooltip
                  v-if="nodeServiceHealthText(row)"
                  placement="top"
                >
                  <template #content>
                    <div class="service-tooltip">
                      <div>{{ nodeServiceHealthTooltipTitle(row) }}</div>
                      <div
                        v-for="line in nodeServiceHealthTooltipLines(row)"
                        :key="`${row.node_id}-${line}`"
                      >
                        {{ line }}
                      </div>
                    </div>
                  </template>
                  <el-tag
                    size="small"
                    effect="plain"
                    :type="nodeServiceHealthTagType(row)"
                    class="node-status-tag"
                    :class="{ 'node-status-tag-clickable': canClickNodeServiceTag(row) }"
                    @click="onNodeServiceTagClick(row)"
                  >
                    {{ nodeServiceHealthText(row) }}
                  </el-tag>
                </el-tooltip>
                <el-tag v-if="row.ssh_exclusive_enabled" size="small" type="danger" effect="dark" class="node-status-tag">
                  独享中
                </el-tag>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="策略" width="340">
          <template #default="{ row }">
            <div class="node-guard-switch-wrap">
              <div class="node-switch-item">
                <el-text size="small" class="node-switch-label">SSH拦截</el-text>
                <el-tooltip
                  content="关闭时仅黑名单拦截；开启后未注册账号会被拦截。开启会清空当前 SSH 会话。"
                  placement="top"
                >
                  <el-switch
                    :model-value="!!row.ssh_guard_enabled"
                    size="small"
                    inline-prompt
                    active-text="开"
                    inactive-text="关"
                    :loading="guardUpdatingNodeId === row.node_id"
                    :disabled="!canManageNodes || guardUpdatingNodeId === row.node_id"
                    @change="(v: boolean) => onNodeSSHGuardToggle(row, v)"
                  />
                </el-tooltip>
              </div>
              <div class="node-switch-item">
                <el-text size="small" class="node-switch-label">积分拦截</el-text>
                <el-tooltip content="开启才扣分并限速；关闭则不扣分且不限速。" placement="top">
                  <el-switch
                    :model-value="!!row.points_intercept_enabled"
                    size="small"
                    inline-prompt
                    active-text="开"
                    inactive-text="关"
                    :loading="pointsUpdatingNodeId === row.node_id"
                    :disabled="!canManageNodes || pointsUpdatingNodeId === row.node_id"
                    @change="(v: boolean) => onNodePointsInterceptToggle(row, v)"
                  />
                </el-tooltip>
              </div>
              <div class="node-switch-item">
                <el-text size="small" class="node-switch-label">硬盘配额</el-text>
                <el-tooltip content="开启后可按节点策略下发磁盘软/硬配额；关闭时不自动下发。" placement="top">
                  <el-switch
                    :model-value="!!row.disk_quota_enabled"
                    size="small"
                    inline-prompt
                    active-text="开"
                    inactive-text="关"
                    :loading="diskQuotaUpdatingNodeId === row.node_id"
                    :disabled="!canManageNodes || diskQuotaUpdatingNodeId === row.node_id"
                    @change="(v: boolean) => onNodeDiskQuotaToggle(row, v)"
                  />
                </el-tooltip>
              </div>
              <div class="node-switch-actions">
                <el-button
                  v-if="canManageNodes"
                  size="small"
                  link
                  type="primary"
                  class="node-visibility-btn"
                  @click="openNodePointsPolicyDialog(row)"
                >
                  限速策略
                </el-button>
                <el-button
                  v-if="canManageNodes"
                  size="small"
                  link
                  type="primary"
                  class="node-visibility-btn"
                  @click="openNodeDiskQuotaDialog(row)"
                >
                  硬盘配额
                </el-button>
                <el-button
                  v-if="canManageNodes"
                  size="small"
                  link
                  type="primary"
                  class="node-visibility-btn"
                  @click="openNodePriceDialog(row)"
                >
                  计费参数
                </el-button>
                <el-button
                  v-if="canManageNodes"
                  size="small"
                  link
                  type="primary"
                  class="node-visibility-btn"
                  @click="openNodeExclusiveDialog(row)"
                >
                  独享
                </el-button>
                <el-button
                  v-if="canManageNodes"
                  size="small"
                  link
                  type="primary"
                  class="node-visibility-btn"
                  @click="openNodeVisibilityDialog(row)"
                >
                  可见
                </el-button>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="last_seen_at" label="最后在线时间" width="200">
          <template #default="{ row }">
            <div class="time-cell">
              <el-icon><Clock /></el-icon>
              <span>{{ formatTime(row.last_seen_at) }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="gpu_process_count" label="GPU 进程" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="row.gpu_process_count > 0 ? 'success' : 'info'" effect="dark" round>
              {{ row.gpu_process_count }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="cpu_process_count" label="CPU 进程" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="row.cpu_process_count > 0 ? 'success' : 'info'" effect="dark" round>
              {{ row.cpu_process_count }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="usage_records_count" label="记录数" width="100" align="center">
          <template #default="{ row }">
            <el-text type="primary" style="font-weight: 600">{{ row.usage_records_count }}</el-text>
          </template>
        </el-table-column>

        <el-table-column prop="cost_total" label="当次积分" width="120" align="right">
          <template #default="{ row }">
            <div class="cost-cell">
              <el-icon color="var(--warning-color)"><Coin /></el-icon>
              <span>{{ row.cost_total.toFixed(4) }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="interval_seconds" label="周期 (秒)" width="110" align="center">
          <template #default="{ row }">
            <el-tag type="info" effect="plain" size="small">{{ row.interval_seconds }}s</el-tag>
          </template>
        </el-table-column>

        <el-table-column label="CPU 信息" min-width="200">
          <template #default="{ row }">
            <el-text>{{ row.cpu_model || "-" }} ({{ row.cpu_count || 0 }})</el-text>
          </template>
        </el-table-column>

        <el-table-column label="GPU 信息" min-width="200">
          <template #default="{ row }">
            <el-text>{{ row.gpu_model || "-" }} ({{ row.gpu_count || 0 }})</el-text>
          </template>
        </el-table-column>

        <el-table-column label="系统版本" min-width="180">
          <template #default="{ row }">
            <el-text>{{ row.os_version || "-" }}</el-text>
          </template>
        </el-table-column>

        <el-table-column label="内核版本" min-width="160">
          <template #default="{ row }">
            <el-text>{{ row.kernel_version || "-" }}</el-text>
          </template>
        </el-table-column>

        <el-table-column label="Agent版本" min-width="210">
          <template #default="{ row }">
            <div class="agent-version-cell">
              <el-text>{{ displayAgentVersion(row.agent_version) }}</el-text>
              <el-tooltip v-if="isAgentVersionOutdated(row)" :content="agentVersionOutdatedTip(row)" placement="top">
                <el-icon class="agent-version-outdated-icon"><WarningFilled /></el-icon>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="根分区 / (总/已用)" min-width="190" align="right">
          <template #default="{ row }">
            <el-text>{{ formatDiskUsage(row.disk_total_gb, row.disk_used_gb) }}</el-text>
          </template>
        </el-table-column>

        <el-table-column label="/home (总/已用)" min-width="190" align="right">
          <template #default="{ row }">
            <el-text>{{ formatDiskUsage(row.home_total_gb, row.home_used_gb) }}</el-text>
          </template>
        </el-table-column>

        <el-table-column label="/mnt (总/已用)" min-width="190" align="right">
          <template #default="{ row }">
            <el-text>{{ formatDiskUsage(row.mnt_total_gb, row.mnt_used_gb) }}</el-text>
          </template>
        </el-table-column>

        <el-table-column label="月流量下行(MB)" width="150" align="right">
          <template #default="{ row }">{{ (row.net_rx_mb_month || 0).toFixed(2) }}</template>
        </el-table-column>

        <el-table-column label="月流量上行(MB)" width="150" align="right">
          <template #default="{ row }">{{ (row.net_tx_mb_month || 0).toFixed(2) }}</template>
        </el-table-column>

        <el-table-column label="SSH在线数" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="(row.ssh_active_count || 0) > 0 ? 'warning' : 'info'" effect="plain">
              {{ row.ssh_active_count || 0 }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="last_report_id" label="Report ID" min-width="200">
          <template #default="{ row }">
            <el-text type="info" size="small" style="font-family: monospace">
              {{ row.last_report_id }}
            </el-text>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="340" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openNodeDetail(row)">详情</el-button>
            <el-button
              size="small"
              type="primary"
              class="sync-status-btn"
              :loading="syncingNodeId === row.node_id"
              :disabled="!canManageNodes"
              @click="syncNodeNow(row.node_id)"
            >
              同步
            </el-button>
            <el-button
              size="small"
              type="danger"
              :loading="disconnectingNodeId === row.node_id"
              :disabled="!canManageNodes"
              @click="disconnectAllSSH(row)"
            >
              踢SSH
            </el-button>
            <el-button
              size="small"
              type="danger"
              plain
              :loading="killingProcNodeId === row.node_id"
              :disabled="!canManageNodes"
              @click="killAllUserProcesses(row)"
            >
              清进程
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-drawer
      v-model="detailVisible"
      :title="`节点详情：${detailNodeId || '-'}`"
      size="78%"
      :destroy-on-close="false"
      @closed="stopDetailAutoRefresh"
    >
      <el-alert v-if="detailError" :title="detailError" type="error" show-icon class="error-alert" />
      <el-skeleton v-if="detailLoading" :rows="6" animated />
      <template v-else-if="detailData">
        <div class="detail-actions">
          <el-text type="info" size="small" class="refresh-time-text">详情刷新：{{ detailRefreshTimeText }}</el-text>
          <el-tooltip content="立即刷新（并重置5分钟自动刷新计时）" placement="top">
            <el-button link class="icon-action-btn" @click="refreshDetailNow">
              <el-icon :size="20"><Refresh /></el-icon>
            </el-button>
          </el-tooltip>
        </div>

        <el-descriptions :column="3" border>
          <el-descriptions-item label="节点ID">{{ detailData.node.node_id }}</el-descriptions-item>
          <el-descriptions-item label="节点IP">{{ detailData.node.node_ip || "-" }}</el-descriptions-item>
          <el-descriptions-item label="节点MAC">{{ detailData.node.node_mac || "-" }}</el-descriptions-item>
          <el-descriptions-item label="最后心跳">{{ formatTime(detailData.node.last_seen_at) }}</el-descriptions-item>
          <el-descriptions-item label="最新快照">{{ formatTime(detailData.latest.report_ts) }}</el-descriptions-item>
          <el-descriptions-item label="服务巡检">
            <div class="node-service-detail">
              <el-tag
                v-if="nodeServiceHealthText(detailData.node)"
                size="small"
                effect="plain"
                :type="nodeServiceHealthTagType(detailData.node)"
              >
                {{ nodeServiceHealthText(detailData.node) }}
              </el-tag>
              <span class="node-service-detail-time">巡检于 {{ formatTime(detailData.node.system_services_checked_at) }}</span>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="CPU型号">{{ detailData.node.cpu_model || "-" }}</el-descriptions-item>
          <el-descriptions-item label="CPU数量">{{ detailData.node.cpu_count || 0 }}</el-descriptions-item>
          <el-descriptions-item label="CPU利用率(估算)">{{ cpuUtilNow.toFixed(2) }}%</el-descriptions-item>
          <el-descriptions-item label="GPU型号">{{ detailData.node.gpu_model || "-" }}</el-descriptions-item>
          <el-descriptions-item label="GPU数量">{{ detailData.node.gpu_count || 0 }}</el-descriptions-item>
          <el-descriptions-item label="GPU活跃度(估算)">{{ gpuUtilNow.toFixed(2) }}%</el-descriptions-item>
          <el-descriptions-item label="系统版本">{{ detailData.node.os_version || "-" }}</el-descriptions-item>
          <el-descriptions-item label="内核版本">{{ detailData.node.kernel_version || "-" }}</el-descriptions-item>
          <el-descriptions-item label="Agent版本">
            <div class="agent-version-cell">
              <span>{{ displayAgentVersion(detailData.node.agent_version) }}</span>
              <el-tooltip v-if="isAgentVersionOutdated(detailData.node)" :content="agentVersionOutdatedTip(detailData.node)" placement="top">
                <el-icon class="agent-version-outdated-icon"><WarningFilled /></el-icon>
              </el-tooltip>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="节点本地用户数">{{ (detailData.local_users || []).length }}</el-descriptions-item>
          <el-descriptions-item label="根分区 / 总容量">{{ fmtGB(detailData.node.disk_total_gb) }}</el-descriptions-item>
          <el-descriptions-item label="根分区 / 已用容量">{{ fmtGB(detailData.node.disk_used_gb) }}</el-descriptions-item>
          <el-descriptions-item label="根分区 / 已用占比">{{ diskUsagePercent(detailData.node.disk_total_gb, detailData.node.disk_used_gb) }}</el-descriptions-item>
          <el-descriptions-item label="/home 总容量">{{ fmtGB(detailData.node.home_total_gb) }}</el-descriptions-item>
          <el-descriptions-item label="/home 已用容量">{{ fmtGB(detailData.node.home_used_gb) }}</el-descriptions-item>
          <el-descriptions-item label="/home 已用占比">{{ diskUsagePercent(detailData.node.home_total_gb, detailData.node.home_used_gb) }}</el-descriptions-item>
          <el-descriptions-item label="/mnt 总容量">{{ fmtGB(detailData.node.mnt_total_gb) }}</el-descriptions-item>
          <el-descriptions-item label="/mnt 已用容量">{{ fmtGB(detailData.node.mnt_used_gb) }}</el-descriptions-item>
          <el-descriptions-item label="/mnt 已用占比">{{ diskUsagePercent(detailData.node.mnt_total_gb, detailData.node.mnt_used_gb) }}</el-descriptions-item>
          <el-descriptions-item label="内存占用总和(MB)">{{ (detailData.latest.memory_mb_sum || 0).toFixed(2) }}</el-descriptions-item>
          <el-descriptions-item label="SSH在线数">{{ detailData.latest.ssh_user_count || 0 }}</el-descriptions-item>
          <el-descriptions-item label="当次积分">{{ (detailData.latest.cost_total || 0).toFixed(4) }}</el-descriptions-item>
          <el-descriptions-item label="节点计费单价">
            <div class="node-price-inline">
              <el-tag v-if="detailData.node.node_price_per_minute != null" type="warning" effect="plain">
                自定义 {{ Number(detailData.node.node_price_per_minute || 0).toFixed(4) }} / GPU·分钟
              </el-tag>
              <el-tag v-else type="info" effect="plain">未设置节点单价</el-tag>
              <el-button
                v-if="canManageNodes"
                size="small"
                link
                type="primary"
                @click="openNodePriceDialog(detailData.node)"
              >
                设置
              </el-button>
            </div>
          </el-descriptions-item>
        </el-descriptions>

        <div class="ssh-users-wrap">
          <div class="ssh-users-title section-inline-title">
            <el-icon><User /></el-icon>
            <span>当前 SSH 登录用户</span>
          </div>
          <el-table
            v-if="sshOnlineRows.length > 0"
            :data="sshOnlineRows"
            size="small"
            stripe
            style="width: 100%"
            :header-cell-style="{ background: 'var(--bg-tertiary)', color: 'var(--text-primary)' }"
          >
            <el-table-column prop="local_username" label="节点账号" min-width="180" />
            <el-table-column label="平台账号" min-width="180">
              <template #default="{ row }">
                <el-tag v-if="row.loading" type="info" effect="plain">解析中</el-tag>
                <el-button
                  v-else-if="row.platform_exists"
                  link
                  type="primary"
                  @click="openPlatformProfile(row.platform_username)"
                >
                  {{ row.platform_username || "-" }}
                </el-button>
                <el-tag v-else type="danger" effect="plain">未在平台开通</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="真实姓名" min-width="120">
              <template #default="{ row }">
                <span v-if="row.loading">-</span>
                <span v-else>{{ row.real_name || "-" }}</span>
              </template>
            </el-table-column>
            <el-table-column label="映射状态" min-width="140">
              <template #default="{ row }">
                <el-tag v-if="row.loading" type="info" effect="plain">解析中</el-tag>
                <el-tag v-else-if="row.mapping_exists" type="success" effect="plain">已映射</el-tag>
                <el-tag v-else type="warning" effect="plain">未映射</el-tag>
                <el-tooltip v-if="!row.loading && row.admin_mapping" content="管理员映射" placement="top">
                  <span class="admin-map-icon" aria-label="管理员映射">管</span>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column prop="message" label="说明" min-width="220" />
            <el-table-column label="操作" width="130" fixed="right">
              <template #default="{ row }">
                <el-button
                  type="danger"
                  size="small"
                  plain
                  :loading="disconnectingUserKey === `${detailNodeId}::${row.local_username}`"
                  :disabled="row.loading || !canManageNodes || (authState.role === 'power_user' && !!row.admin_mapping)"
                  @click="disconnectSSHUser(row.local_username)"
                >
                  强制下线
                </el-button>
              </template>
            </el-table-column>
          </el-table>
          <div v-else class="ssh-users-empty">
            当前没有 SSH 在线用户
            <span v-if="(detailData.latest.ssh_user_count || 0) > 0">（节点仅上报了人数，未上报节点账号，请更新 node-agent）</span>
          </div>
        </div>

        <div class="ssh-users-wrap">
          <div class="section-inline-title-row">
            <div class="ssh-users-title section-inline-title">
              <el-icon><UserFilled /></el-icon>
              <span>节点内全部本地用户（含映射状态）</span>
            </div>
            <el-button
              type="danger"
              size="small"
              plain
              :loading="killingProcNodeId === detailNodeId"
              :disabled="!canManageNodes || !detailNodeId || (detailData.local_users || []).length === 0 || detailHasProtectedAdminMappings"
              @click="killAllDetailUsersProcesses"
            >
              强制清除全部用户进程
            </el-button>
          </div>
          <el-table
            :data="detailData.local_users || []"
            size="small"
            stripe
            style="width: 100%"
            :header-cell-style="{ background: 'var(--bg-tertiary)', color: 'var(--text-primary)' }"
            empty-text="暂无节点用户数据（请更新 node-agent 并等待上报）"
          >
            <el-table-column prop="local_username" label="节点账号" min-width="180" />
            <el-table-column label="映射状态" min-width="120">
              <template #default="{ row }">
                <el-tag v-if="row.mapping_exists" type="success" effect="plain">已映射</el-tag>
                <el-tag v-else type="warning" effect="plain">未映射</el-tag>
                <el-tooltip v-if="row.admin_mapping" content="管理员映射" placement="top">
                  <span class="admin-map-icon" aria-label="管理员映射">管</span>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column label="平台账号" min-width="180">
              <template #default="{ row }">
                <el-button
                  v-if="row.mapping_exists && row.platform_username"
                  link
                  type="primary"
                  @click="openPlatformProfile(row.platform_username)"
                >
                  {{ row.platform_username }}
                </el-button>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column label="创建时间" min-width="170">
              <template #default="{ row }">{{ formatTime(row.home_created_at) }}</template>
            </el-table-column>
            <el-table-column label="最近登录" min-width="170">
              <template #default="{ row }">{{ formatTime(row.last_login_at) }}</template>
            </el-table-column>
            <el-table-column label="sudo权限" min-width="110" align="center">
              <template #default="{ row }">
                <el-tag :type="row.has_sudo ? 'danger' : 'info'" effect="plain">
                  {{ row.has_sudo ? "有" : "无" }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="docker权限" min-width="120" align="center">
              <template #default="{ row }">
                <el-tag :type="row.has_docker ? 'warning' : 'info'" effect="plain">
                  {{ row.has_docker ? "有" : "无" }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="/home占用" min-width="130" align="left">
              <template #default="{ row }">
                <span class="home-used-cell" :class="{ 'home-used-danger': Number(row.home_used_gb || 0) > 50 }">
                  {{ fmtGB(row.home_used_gb) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="当前限制" min-width="220">
              <template #default="{ row }">
                <div class="cpu-limit-cell">
                  <div class="cpu-limit-tag-row">
                    <el-tag v-if="typeof row.cpu_quota_percent === 'number' && Number(row.cpu_quota_percent) > 0" type="warning" effect="dark">
                      CPU {{ Number(row.cpu_quota_percent).toFixed(1) }}%
                    </el-tag>
                    <el-tag v-if="typeof row.memory_limit_gb === 'number' && Number(row.memory_limit_gb) > 0" type="danger" effect="dark">
                      内存 {{ Number(row.memory_limit_gb).toFixed(1) }} GB
                    </el-tag>
                    <el-tag v-if="Array.isArray(row.gpu_visible_indices) && row.gpu_visible_indices.length > 0" type="primary" effect="dark">
                      GPU 可见 {{ formatGPUIndices(row.gpu_visible_indices) }}
                    </el-tag>
                    <el-tag v-if="!(typeof row.cpu_quota_percent === 'number' && Number(row.cpu_quota_percent) > 0) && !(typeof row.memory_limit_gb === 'number' && Number(row.memory_limit_gb) > 0) && !(Array.isArray(row.gpu_visible_indices) && row.gpu_visible_indices.length > 0)" type="info" effect="plain">
                      未限制
                    </el-tag>
                  </div>
                  <div v-if="row.cpu_quota_reason" class="cpu-limit-reason">CPU：{{ row.cpu_quota_reason }}</div>
                  <div v-if="row.memory_limit_reason" class="cpu-limit-reason">内存：{{ row.memory_limit_reason }}</div>
                  <div v-if="row.gpu_visibility_reason" class="cpu-limit-reason">GPU：{{ row.gpu_visibility_reason }}</div>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="限制操作" min-width="220">
              <template #default="{ row }">
                <div class="cpu-limit-editor">
                  <el-button
                    type="warning"
                    size="small"
                    :loading="userLimitSaving && userLimitDialogVisible && userLimitLocalUsername === row.local_username"
                    :disabled="!canManageNodes || !detailNodeId || !row.local_username || isProtectedAdminMapping(row)"
                    @click="openDetailUserLimitDialog(row)"
                  >
                    限制
                  </el-button>
                  <el-button
                    type="info"
                    plain
                    size="small"
                    :loading="userLimitRemovingKey === `${detailNodeId}::${row.local_username}`"
                    :disabled="!canManageNodes || !detailNodeId || !row.local_username || !hasManualRestriction(row) || isProtectedAdminMapping(row)"
                    @click="clearDetailUserLimits(row.local_username)"
                  >
                    解除
                  </el-button>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="120" fixed="right">
              <template #default="{ row }">
                <el-button
                  type="danger"
                  size="small"
                  plain
                  :loading="killingUserProcKey === `${detailNodeId}::${row.local_username}`"
                  :disabled="!canManageNodes || !detailNodeId || !row.local_username || isProtectedAdminMapping(row)"
                  @click="killDetailUserProcesses(row.local_username)"
                >
                  清进程
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="ssh-users-wrap">
          <div class="ssh-users-title section-inline-title">
            <el-icon><Coin /></el-icon>
            <span>本节点当月积分消耗（按平台账号汇总）</span>
            <el-text type="info" size="small" style="margin-left: 8px">
              {{ formatTime(detailData.monthly_from || "") }} ~ {{ formatTime(detailData.monthly_to || "") }}
            </el-text>
          </div>
          <el-table
            :data="detailData.monthly_user_costs || []"
            size="small"
            stripe
            style="width: 100%"
            :header-cell-style="{ background: 'var(--bg-tertiary)', color: 'var(--text-primary)' }"
            empty-text="本月暂无积分消耗记录"
          >
            <el-table-column label="平台账号" min-width="180">
              <template #default="{ row }">
                <el-button link type="primary" @click="openPlatformProfile(row.platform_username)">
                  {{ row.platform_username }}
                </el-button>
              </template>
            </el-table-column>
            <el-table-column prop="usage_records" label="记录数" width="110" align="center" />
            <el-table-column label="当月消耗积分" min-width="140" align="right">
              <template #default="{ row }">
                <el-text type="warning" style="font-weight: 700">{{ Number(row.total_cost || 0).toFixed(4) }}</el-text>
              </template>
            </el-table-column>
            <el-table-column label="最近使用时间" min-width="180">
              <template #default="{ row }">{{ formatTime(row.last_usage_at) }}</template>
            </el-table-column>
          </el-table>
        </div>

        <div class="ssh-users-wrap">
          <div class="ssh-users-title section-inline-title">
            <el-icon><WarningFilled /></el-icon>
            <span>疑似恶意用户名单（近7天自动统计）</span>
          </div>
          <el-table
            :data="detailData.suspicious_users || []"
            size="small"
            stripe
            style="width: 100%"
            :header-cell-style="{ background: 'var(--bg-tertiary)', color: 'var(--text-primary)' }"
            empty-text="暂无可疑账号"
          >
            <el-table-column prop="username" label="节点账号" min-width="180" />
            <el-table-column prop="hit_count" label="触发次数" width="110" align="center" />
            <el-table-column label="最近触发时间" min-width="180">
              <template #default="{ row }">{{ formatTime(row.last_seen_at) }}</template>
            </el-table-column>
            <el-table-column label="疑似现象" min-width="240">
              <template #default="{ row }">
                <span>{{ row.phenomena || "-" }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="reason_hints" label="自动理由" min-width="280" />
            <el-table-column label="疑似挖矿" width="110" align="center">
              <template #default="{ row }">
                <el-tag v-if="row.mining_suspected" type="danger" effect="dark">是</el-tag>
                <el-tag v-else type="info" effect="plain">否</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="260" fixed="right">
              <template #default="{ row }">
                <el-button size="small" @click="showSuspiciousDetail(row)">查看详细</el-button>
                <el-button
                  size="small"
                  type="danger"
                  plain
                  :loading="blacklistingUserKey === `${detailNodeId}::${row.username}`"
                  @click="blacklistSuspiciousUser(row)"
                >
                  拉黑
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="ssh-users-wrap">
          <div class="ssh-users-title section-inline-title">
            <el-icon><Document /></el-icon>
            <span>安全审计日志（节点维度）</span>
          </div>
          <div class="security-filter-bar">
            <el-date-picker
              v-model="securityRange"
              type="datetimerange"
              start-placeholder="开始时间"
              end-placeholder="结束时间"
              format="YYYY-MM-DD HH:mm:ss"
              value-format="YYYY-MM-DD HH:mm:ss"
              range-separator="至"
              style="width: 420px"
            />
            <el-select v-model="securityEventTypeFilter" clearable filterable placeholder="事件类型（可选）" style="width: 220px">
              <el-option label="全部事件" value="" />
              <el-option label="疑似挖矿" value="suspected_mining" />
              <el-option label="高CPU负载" value="high_cpu_load" />
              <el-option label="SSH失败峰值" value="ssh_failed_login_spike" />
              <el-option label="SSH爆破" value="ssh_bruteforce" />
              <el-option label="端口扫描" value="abnormal_port_scan" />
              <el-option label="磁盘风险" value="disk_full_risk" />
            </el-select>
            <el-switch v-model="securityShowSummary" inline-prompt active-text="规约视图" inactive-text="原始日志" />
            <el-button type="primary" :loading="securityEventsLoading" @click="queryNodeSecurityEvents">查询</el-button>
            <el-button :disabled="securityEventsLoading" @click="resetNodeSecurityFilters">重置</el-button>
          </div>
          <el-alert
            type="info"
            show-icon
            :closable="false"
            class="security-normalizer-alert"
            :title="`规约说明：${securitySummaryNormalizer || 'event_type + severity + reason(数字归一化)'}`"
          />
          <el-table
            v-if="securityShowSummary"
            :data="securityEventSummariesRows"
            size="small"
            stripe
            style="width: 100%"
            v-loading="securityEventsLoading"
            :header-cell-style="{ background: 'var(--bg-tertiary)', color: 'var(--text-primary)' }"
            empty-text="当前时间范围内暂无可规约的安全事件"
          >
            <el-table-column prop="event_type" label="事件类型" min-width="140" />
            <el-table-column prop="severity" label="等级" width="90" />
            <el-table-column prop="normalized_reason" label="规约后原因" min-width="280" />
            <el-table-column prop="event_count" label="事件数" width="90" align="center" />
            <el-table-column prop="affected_users" label="影响账号数" width="110" align="center" />
            <el-table-column label="首次时间" min-width="170">
              <template #default="{ row }">{{ formatTime(row.first_seen_at) }}</template>
            </el-table-column>
            <el-table-column label="最近时间" min-width="170">
              <template #default="{ row }">{{ formatTime(row.last_seen_at) }}</template>
            </el-table-column>
            <el-table-column label="详情" width="130" fixed="right">
              <template #default="{ row }">
                <el-button size="small" @click="showSecuritySummaryDetail(row)">查看</el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-table
            v-else
            :data="securityEventsRows"
            size="small"
            stripe
            style="width: 100%"
            v-loading="securityEventsLoading"
            :header-cell-style="{ background: 'var(--bg-tertiary)', color: 'var(--text-primary)' }"
            empty-text="暂无安全事件日志"
          >
            <el-table-column label="时间" min-width="170">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column prop="event_type" label="事件类型" min-width="120" />
            <el-table-column prop="severity" label="等级" width="90" />
            <el-table-column prop="reason" label="原因" min-width="280" />
            <el-table-column label="相关账号" min-width="180">
              <template #default="{ row }">
                <span>{{ (row.related_usernames || []).join(", ") || "-" }}</span>
              </template>
            </el-table-column>
            <el-table-column label="详情" width="120" fixed="right">
              <template #default="{ row }">
                <el-button size="small" @click="showSecurityEventDetail(row)">查看</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <div class="charts-grid">
          <el-card class="chart-card">
            <template #header>
              <div class="chart-head">
                <el-icon><Cpu /></el-icon>
                <span>CPU 利用率趋势（估算）</span>
              </div>
            </template>
            <div class="chart-value">{{ cpuUtilNow.toFixed(2) }}%</div>
            <svg class="chart-svg" viewBox="0 0 520 130" preserveAspectRatio="none">
              <polyline :points="cpuLinePoints" fill="none" stroke="#0ea5e9" stroke-width="3" />
            </svg>
          </el-card>
          <el-card class="chart-card">
            <template #header>
              <div class="chart-head">
                <el-icon><Monitor /></el-icon>
                <span>内存占用趋势（MB）</span>
              </div>
            </template>
            <div class="chart-value">{{ (detailData.latest.memory_mb_sum || 0).toFixed(2) }}</div>
            <svg class="chart-svg" viewBox="0 0 520 130" preserveAspectRatio="none">
              <polyline :points="memoryLinePoints" fill="none" stroke="#14b8a6" stroke-width="3" />
            </svg>
          </el-card>
          <el-card class="chart-card">
            <template #header>
              <div class="chart-head">
                <el-icon><User /></el-icon>
                <span>SSH 在线人数趋势</span>
              </div>
            </template>
            <div class="chart-value">{{ detailData.latest.ssh_user_count || 0 }}</div>
            <svg class="chart-svg" viewBox="0 0 520 130" preserveAspectRatio="none">
              <polyline :points="sshLinePoints" fill="none" stroke="#f59e0b" stroke-width="3" />
            </svg>
          </el-card>
          <el-card class="chart-card">
            <template #header>
              <div class="chart-head">
                <el-icon><List /></el-icon>
                <span>GPU 进程数趋势</span>
              </div>
            </template>
            <div class="chart-value">{{ detailData.latest.gpu_process_count || 0 }}</div>
            <svg class="chart-svg" viewBox="0 0 520 130" preserveAspectRatio="none">
              <polyline :points="gpuProcLinePoints" fill="none" stroke="#8b5cf6" stroke-width="3" />
            </svg>
          </el-card>
        </div>
      </template>
    </el-drawer>

    <el-dialog v-model="platformProfileVisible" title="平台账号注册信息" width="640px">
      <el-alert v-if="platformProfileError" :title="platformProfileError" type="error" show-icon class="error-alert" />
      <el-skeleton v-else-if="platformProfileLoading" :rows="6" animated />
      <el-descriptions v-else-if="platformProfile" :column="2" border>
        <el-descriptions-item label="平台账号">{{ platformProfile.username }}</el-descriptions-item>
        <el-descriptions-item label="真实姓名">{{ platformProfile.real_name || "-" }}</el-descriptions-item>
        <el-descriptions-item label="学号">{{ platformProfile.student_id || "-" }}</el-descriptions-item>
        <el-descriptions-item label="邮箱">{{ platformProfile.email || "-" }}</el-descriptions-item>
        <el-descriptions-item label="导师">{{ platformProfile.advisor || "-" }}</el-descriptions-item>
        <el-descriptions-item label="电话">{{ platformProfile.phone || "-" }}</el-descriptions-item>
        <el-descriptions-item label="预计毕业">{{ `${platformProfile.expected_graduation_year || "-"}-${String(platformProfile.expected_graduation_month || "").padStart(2, "0")}` }}</el-descriptions-item>
        <el-descriptions-item label="积分余额">{{ Number(platformProfile.balance || 0).toFixed(2) }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ platformProfile.status || "-" }}</el-descriptions-item>
        <el-descriptions-item label="角色">{{ platformProfile.role || "-" }}</el-descriptions-item>
      </el-descriptions>
      <template v-if="platformProfile">
        <div class="ssh-users-title">节点账号映射</div>
        <el-table :data="platformProfile.node_accounts || []" stripe max-height="220" empty-text="暂无映射">
          <el-table-column prop="node_id" label="节点编号" width="140" />
          <el-table-column prop="local_username" label="节点账号" width="160" />
          <el-table-column label="状态" width="220">
            <template #default="{ row }">
              <div class="mapping-state-cell">
                <el-tag v-if="row.identity_aligned" type="success" effect="light">已就绪</el-tag>
                <el-tag v-else-if="row.identity_initializing" type="warning" effect="light">初始化中</el-tag>
                <el-tag v-else type="info" effect="light">待同步</el-tag>
                <div v-if="mappingStateTip(row)" class="mapping-state-tip">{{ mappingStateTip(row) }}</div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="更新时间" min-width="180">
            <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
          </el-table-column>
        </el-table>
      </template>
    </el-dialog>

    <el-dialog v-model="nodeVisibilityVisible" title="节点可见性设置" width="560px">
      <el-form label-width="110px">
        <el-form-item label="节点编号">
          <el-text>{{ nodeVisibilityNodeId || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="可见范围">
          <el-switch
            v-model="nodeVisibilityRestricted"
            inline-prompt
            active-text="限制"
            inactive-text="全部"
          />
          <el-text type="info" size="small" style="margin-left: 8px">
            关闭时：所有高级用户可见；开启时：仅下方选中的高级用户可见（管理员始终可见）
          </el-text>
        </el-form-item>
        <el-form-item label="指定高级用户" v-if="nodeVisibilityRestricted">
          <el-select
            v-model="nodeVisibilityAllowedUsers"
            multiple
            filterable
            clearable
            style="width: 100%"
            placeholder="选择可查看该节点的高级用户"
          >
            <el-option v-for="u in nodeVisibilityCandidates" :key="u" :label="u" :value="u" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="nodeVisibilityRestricted && nodeVisibilityCandidates.length === 0">
          <el-alert
            type="info"
            show-icon
            :closable="false"
            title="当前没有可分配的高级用户（或暂无节点查看权限的高级用户）。你可以先在“高级用户管理”新增/授权后再设置。"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="nodeVisibilityVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodeVisibilitySaving" @click="saveNodeVisibility">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="nodeExclusiveVisible" title="节点独享设置（SSH/GPU）" width="760px">
      <el-form label-width="110px">
        <el-form-item label="节点编号">
          <el-text>{{ nodeExclusiveNodeId || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="独享开关">
          <el-switch v-model="nodeExclusiveEnabled" inline-prompt active-text="开启" inactive-text="关闭" />
          <el-text type="info" size="small" style="margin-left: 8px">
            开启后可按用户分配可见 GPU 卡；可选“是否封锁其他用户 SSH”
          </el-text>
        </el-form-item>
        <el-form-item label="封锁其他用户SSH" v-if="nodeExclusiveEnabled">
          <el-switch v-model="nodeExclusiveBlockOtherSSH" inline-prompt active-text="封锁" inactive-text="不封锁" />
          <el-text type="info" size="small" style="margin-left: 8px">
            开启后，仅“独享用户 + 白名单 + 豁免”可登录 SSH；关闭时仅做 GPU 卡可见性隔离
          </el-text>
        </el-form-item>
        <el-form-item label="独享用户" v-if="nodeExclusiveEnabled">
          <el-select
            v-model="nodeExclusiveUsers"
            multiple
            filterable
            clearable
            style="width: 100%"
            placeholder="选择可独享该节点的内部用户"
          >
            <el-option v-for="u in nodeExclusiveCandidates" :key="u" :label="u" :value="u" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="nodeExclusiveEnabled && nodeExclusiveCandidates.length === 0">
          <el-alert
            type="info"
            show-icon
            :closable="false"
            title="暂无可选节点内部用户。可先等待节点上报本地用户，或先创建节点账号映射后再设置。"
          />
        </el-form-item>
        <el-form-item label="独享卡分配" v-if="nodeExclusiveEnabled && nodeExclusiveUsers.length > 0">
          <div style="width: 100%">
            <el-alert
              v-if="nodeExclusiveGPUCount <= 0"
              type="warning"
              show-icon
              :closable="false"
              title="该节点暂未检测到 GPU 数量，当前无法配置按卡独享。"
            />
            <div v-else class="exclusive-gpu-assign-wrap">
              <div v-for="u in nodeExclusiveUsers" :key="`exclusive-gpu-${u}`" class="exclusive-gpu-user-row">
                <div class="exclusive-gpu-user">{{ u }}</div>
                <el-checkbox-group
                  :model-value="nodeExclusiveGPUAssignments[u] || []"
                  @update:model-value="(v: any) => { nodeExclusiveGPUAssignments[u] = normalizeGPUIndexList((v || []) as number[]); }"
                >
                  <el-checkbox v-for="idx in nodeExclusiveGPUOptions" :key="`gpu-${u}-${idx}`" :label="idx">
                    GPU{{ idx }}
                  </el-checkbox>
                </el-checkbox-group>
              </div>
              <el-text type="info" size="small">
                规则：同一张 GPU 可以同时分配给多个独享用户；未勾选到某用户的 GPU 对该用户不可见，其他非豁免用户只能看到“未分配”的 GPU。
              </el-text>
            </div>
          </div>
        </el-form-item>
        <el-form-item label="规则说明">
          <el-alert
            type="warning"
            show-icon
            :closable="false"
            title="豁免用户始终可见全部 GPU，且不受独享规则影响（无视封锁 SSH 与卡分配）。"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="nodeExclusiveVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodeExclusiveSaving" @click="saveNodeExclusive">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="nodePointsPolicyVisible" title="节点积分限速策略" width="760px">
      <el-form label-width="170px">
        <el-form-item label="节点ID">
          <el-text>{{ nodePointsPolicyNodeId || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="积分拦截开关">
          <el-switch v-model="nodePointsPolicyEnabled" inline-prompt active-text="开启" inactive-text="关闭" />
          <el-text type="info" size="small" style="margin-left: 8px">
            开启后才会扣分并按阈值触发限速；关闭则不扣分且不限速
          </el-text>
        </el-form-item>
        <el-form-item label="低积分限速阈值">
          <el-input-number
            v-model="nodePointsThrottleThreshold"
            :min="0"
            :step="1"
            :precision="2"
            style="width: 220px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">余额 ≤ 阈值时触发“低积分限速”</el-text>
        </el-form-item>
        <el-form-item label="低积分限速比例">
          <el-input-number
            v-model="nodePointsLimitedCPUQuota"
            :min="1"
            :max="100"
            :step="1"
            :precision="1"
            style="width: 220px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">对应 `set_cpu_quota`，单位：%</el-text>
        </el-form-item>
        <el-form-item label="欠费强限速比例">
          <el-input-number
            v-model="nodePointsBlockedCPUQuota"
            :min="1"
            :max="100"
            :step="1"
            :precision="1"
            style="width: 220px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">当余额超过“每月欠费上限”时使用，更严格</el-text>
        </el-form-item>
        <el-form-item label="欠费内存上限(GB)">
          <el-input-number
            v-model="nodePointsOverdraftMemoryGB"
            :min="0"
            :step="0.5"
            :precision="2"
            style="width: 220px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">
            当超过每月欠费上限时生效；0 表示关闭内存上限
          </el-text>
        </el-form-item>
        <el-form-item>
          <el-alert
            type="info"
            show-icon
            :closable="false"
            class="points-policy-help"
          >
            <template #title>可调项说明（避免误操作）</template>
            <div class="points-policy-help-lines">
              <div>1. 积分拦截开关：关闭时本节点不扣分且不触发限速，仅保留使用记录。</div>
              <div>2. 低积分限速阈值：当账号余额 ≤ 该值时，触发“低积分限速比例”。</div>
              <div>3. 低积分/欠费比例：分别对应两种 `set_cpu_quota` 强度，值越小限制越强。</div>
              <div>4. 欠费内存上限：仅在“超过每月欠费上限”时触发 `set_memory_limit`。</div>
              <div>5. 当前控制器 CPU 限速总开关：{{ nodePointsCPUControlEnabled ? "已开启" : "未开启（即使保存也不会生效）" }}。</div>
            </div>
          </el-alert>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="nodePointsPolicyVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodePointsPolicySaving" @click="saveNodePointsPolicy">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="nodeDiskQuotaVisible" title="节点硬盘配额策略" width="980px">
      <el-form label-width="170px" v-loading="nodeDiskQuotaLoading">
        <el-form-item label="节点ID">
          <el-text>{{ nodeDiskQuotaNodeId || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="Quota 安装状态">
          <el-alert
            :type="nodeDiskQuotaInstalled ? 'success' : 'warning'"
            show-icon
            :closable="false"
            :title="nodeDiskQuotaInstalled ? '已检测到 quota 工具（setquota）' : '未检测到 quota 工具（setquota）'"
            class="disk-quota-alert"
          />
        </el-form-item>
        <el-form-item label="已启用 Quota 分区">
          <div class="disk-quota-mounts">
            <el-tag v-for="m in nodeDiskQuotaMounts" :key="`dq-mount-${m}`" type="info" effect="plain">{{ m }}</el-tag>
            <el-text v-if="nodeDiskQuotaMounts.length === 0" type="warning">节点尚未上报启用 quota 的分区（常见是 /home 或 /）</el-text>
          </div>
        </el-form-item>
        <el-form-item label="策略开关">
          <el-switch v-model="nodeDiskQuotaEnabled" inline-prompt active-text="开启" inactive-text="关闭" />
          <el-text type="info" size="small" style="margin-left: 8px">
            开启后可按策略自动下发用户磁盘配额；关闭仅停止自动下发，不会自动清空已有配额
          </el-text>
        </el-form-item>
        <el-form-item label="配额分区">
          <el-select v-model="nodeDiskQuotaMountpoint" clearable style="width: 260px" placeholder="选择 quota 分区">
            <el-option v-for="m in nodeDiskQuotaMounts" :key="`dq-opt-${m}`" :label="m" :value="m" />
          </el-select>
          <el-text type="info" size="small" style="margin-left: 8px">
            当前优先：{{ nodeDiskQuotaPreferredMountpoint || "未检测到" }}
          </el-text>
        </el-form-item>
        <el-form-item label="默认软配额 (G)">
          <el-input-number v-model="nodeDiskQuotaDefaultSoftGB" :min="0" :step="1" :precision="2" style="width: 220px" />
          <el-text type="info" size="small" style="margin-left: 8px">达到软阈值后触发告警/宽限（由系统实现），`0` 表示不限额</el-text>
        </el-form-item>
        <el-form-item label="默认硬配额 (G)">
          <el-input-number v-model="nodeDiskQuotaDefaultHardGB" :min="0" :step="1" :precision="2" style="width: 220px" />
          <el-text type="info" size="small" style="margin-left: 8px">达到硬阈值后禁止继续写入；`0` 表示不限额</el-text>
        </el-form-item>
        <el-form-item label="保存后自动全体应用">
          <el-switch v-model="nodeDiskQuotaApplyAllOnSave" inline-prompt active-text="是" inactive-text="否" />
        </el-form-item>
      </el-form>

      <div class="ssh-users-wrap">
        <div class="ssh-users-title section-inline-title">
          <el-icon><UserFilled /></el-icon>
          <span>用户配额（可逐个调整）</span>
        </div>
        <el-table
          :data="nodeDiskQuotaUsers"
          size="small"
          stripe
          max-height="360"
          style="width: 100%"
          :header-cell-style="{ background: 'var(--bg-tertiary)', color: 'var(--text-primary)' }"
          empty-text="暂无节点本地用户"
        >
          <el-table-column prop="local_username" label="节点账号" min-width="140" />
          <el-table-column label="平台账号" min-width="140">
            <template #default="{ row }">
              <el-button
                v-if="row.platform_username"
                link
                type="primary"
                @click="openPlatformProfile(row.platform_username)"
              >
                {{ row.platform_username }}
              </el-button>
              <span v-else>-</span>
            </template>
          </el-table-column>
          <el-table-column label="当前分区" min-width="100">
            <template #default="{ row }">{{ row.quota_mountpoint || nodeDiskQuotaMountpoint || "-" }}</template>
          </el-table-column>
          <el-table-column label="已用 (G)" min-width="110" align="right">
            <template #default="{ row }">{{ fmtQuotaGBFromMB(row.quota_used_mb) }}</template>
          </el-table-column>
          <el-table-column label="当前软配额 (G)" min-width="130" align="right">
            <template #default="{ row }">{{ fmtQuotaGBFromMB(row.quota_soft_mb, true) }}</template>
          </el-table-column>
          <el-table-column label="当前硬配额 (G)" min-width="130" align="right">
            <template #default="{ row }">{{ fmtQuotaGBFromMB(row.quota_hard_mb, true) }}</template>
          </el-table-column>
          <el-table-column label="新软配额 (G)" min-width="130">
            <template #default="{ row }">
              <el-input-number v-model="row.edit_soft_gb" :min="0" :step="1" :precision="2" style="width: 120px" />
            </template>
          </el-table-column>
          <el-table-column label="新硬配额 (G)" min-width="130">
            <template #default="{ row }">
              <el-input-number v-model="row.edit_hard_gb" :min="0" :step="1" :precision="2" style="width: 120px" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button
                type="primary"
                size="small"
                :loading="diskQuotaApplyingUserKey === `${nodeDiskQuotaNodeId}::${row.local_username}`"
                @click="applyNodeDiskQuotaUser(row)"
              >
                应用
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <template #footer>
        <el-button :loading="nodeDiskQuotaApplyingAll" @click="applyNodeDiskQuotaAll">全体应用默认配额</el-button>
        <el-button @click="nodeDiskQuotaVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodeDiskQuotaSaving" @click="saveNodeDiskQuotaPolicy">保存策略</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="nodePriceVisible" title="节点计费参数设置" width="760px">
      <el-form label-width="130px">
        <el-form-item label="节点ID">
          <el-text>{{ nodePriceNodeId || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="CPU 型号">
          <el-text>{{ nodePriceCPUModel || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="GPU 型号">
          <el-text>{{ nodePriceGPUModel || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="GPU 单价">
          <el-input-number
            v-model="nodePricePerMinute"
            :min="0"
            :step="0.01"
            :precision="4"
            style="width: 220px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">单位：积分 / GPU·分钟</el-text>
        </el-form-item>
        <el-form-item label="CPU 单价">
          <el-input-number
            v-model="nodeCPUPricePerCoreMinute"
            :min="0"
            :step="0.0001"
            :precision="4"
            style="width: 220px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">单位：积分 / 核·分钟</el-text>
        </el-form-item>
        <el-form-item label="默认值">
          <el-text type="info">
            GPU 默认 {{ nodePriceDefaultPerMinute.toFixed(4) }} / GPU·分钟，
            CPU 默认 {{ nodeCPUPriceDefaultPerCoreMinute.toFixed(4) }} / 核·分钟
          </el-text>
        </el-form-item>
        <el-form-item label="说明">
          <el-alert
            type="info"
            show-icon
            :closable="false"
            title="节点计费参数仅对当前节点生效；可分别配置 GPU 与 CPU 单价。"
          />
        </el-form-item>
        <el-form-item label="计费规则">
          <el-alert
            type="info"
            show-icon
            :closable="false"
            :title="nodePriceRuleFormula"
          />
          <div style="margin-top: 8px; width: 100%">
            <el-text type="info" size="small">GPU 单价优先级：{{ nodePriceRuleGPUPriority.join(" > ") }}</el-text>
            <br />
            <el-text type="info" size="small">CPU 单价优先级：{{ nodePriceRuleCPUPriority.join(" > ") }}</el-text>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="nodePriceVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodePriceSaving" @click="saveNodePrice">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="userLimitDialogVisible"
      title="用户限制设置"
      width="620px"
      destroy-on-close
    >
      <el-form label-width="120px">
        <el-form-item label="节点">
          <el-text>{{ userLimitNodeId || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="节点账号">
          <el-text>{{ userLimitLocalUsername || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="平台账号">
          <el-text>{{ userLimitPlatformUsername || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="限制 CPU">
          <el-switch v-model="userLimitCPUEnabled" inline-prompt active-text="是" inactive-text="否" />
          <el-input-number
            v-model="userLimitCPUPercent"
            :disabled="!userLimitCPUEnabled"
            :min="1"
            :max="100"
            :step="1"
            :precision="1"
            style="margin-left: 10px; width: 140px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">单位：%</el-text>
        </el-form-item>
        <el-form-item label="限制内存">
          <el-switch v-model="userLimitMemoryEnabled" inline-prompt active-text="是" inactive-text="否" />
          <el-input-number
            v-model="userLimitMemoryGB"
            :disabled="!userLimitMemoryEnabled"
            :min="0.5"
            :max="4096"
            :step="0.5"
            :precision="1"
            style="margin-left: 10px; width: 140px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">单位：GB</el-text>
        </el-form-item>
        <el-form-item label="限制 GPU 可见">
          <el-switch v-model="userLimitGPUEnabled" inline-prompt active-text="是" inactive-text="否" />
          <div style="margin-left: 10px; width: calc(100% - 140px)">
            <el-checkbox-group v-model="userLimitVisibleGPUIndices" :disabled="!userLimitGPUEnabled || userLimitGPUOptions.length === 0">
              <el-checkbox v-for="idx in userLimitGPUOptions" :key="`limit-gpu-${idx}`" :label="idx">
                GPU {{ idx }}
              </el-checkbox>
            </el-checkbox-group>
            <el-text v-if="userLimitGPUOptions.length === 0" type="info" size="small">当前节点未检测到可配置的 GPU 编号</el-text>
          </div>
        </el-form-item>
        <el-form-item label="限制原因">
          <el-input
            v-model="userLimitReason"
            type="textarea"
            :rows="3"
            clearable
            maxlength="160"
            show-word-limit
            placeholder="可选：例如手动管控/占用过高/违规任务"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="userLimitDialogVisible = false">取消</el-button>
        <el-button type="warning" :loading="userLimitSaving" @click="saveDetailUserLimitsFromDialog">保存限制</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  ApiClient,
  type NodeDetailResp,
  type NodeLocalUser,
  type NodeRuntimeSnapshot,
  type NodeSecurityEvent,
  type NodeSecurityEventSummary,
  type NodeStatus,
  type NodeSystemServiceStatus,
  type NodeSuspiciousUser,
  type UserNodeAccount,
} from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime } from "../../lib/time";
import { Monitor, Refresh, Cpu, Coin, Clock, List, Document, User, UserFilled, WarningFilled } from "@element-plus/icons-vue";
import dayjs from "dayjs";

const loading = ref(false);
const error = ref("");
const rows = ref<NodeStatus[]>([]);
const disconnectingNodeId = ref("");
const killingProcNodeId = ref("");
const killingUserProcKey = ref("");
const userLimitSaving = ref(false);
const userLimitRemovingKey = ref("");
const userLimitDialogVisible = ref(false);
const userLimitNodeId = ref("");
const userLimitLocalUsername = ref("");
const userLimitPlatformUsername = ref("");
const userLimitCPUEnabled = ref(false);
const userLimitCPUPercent = ref(50);
const userLimitMemoryEnabled = ref(false);
const userLimitMemoryGB = ref(8);
const userLimitGPUEnabled = ref(false);
const userLimitVisibleGPUIndices = ref<number[]>([]);
const userLimitReason = ref("");
const syncingNodeId = ref("");
const guardUpdatingNodeId = ref("");
const pointsUpdatingNodeId = ref("");
const diskQuotaUpdatingNodeId = ref("");
const nodeVisibilityVisible = ref(false);
const nodeVisibilitySaving = ref(false);
const nodeVisibilityNodeId = ref("");
const nodeVisibilityRestricted = ref(false);
const nodeVisibilityAllowedUsers = ref<string[]>([]);
const nodeVisibilityCandidates = ref<string[]>([]);
const nodeExclusiveVisible = ref(false);
const nodeExclusiveSaving = ref(false);
const nodeExclusiveNodeId = ref("");
const nodeExclusiveEnabled = ref(false);
const nodeExclusiveBlockOtherSSH = ref(true);
const nodeExclusiveUsers = ref<string[]>([]);
const nodeExclusiveCandidates = ref<string[]>([]);
const nodeExclusiveGPUCount = ref(0);
const nodeExclusiveGPUAssignments = ref<Record<string, number[]>>({});
const nodePointsPolicyVisible = ref(false);
const nodePointsPolicySaving = ref(false);
const nodePointsPolicyNodeId = ref("");
const nodePointsPolicyEnabled = ref(true);
const nodePointsThrottleThreshold = ref(0);
const nodePointsLimitedCPUQuota = ref(40);
const nodePointsBlockedCPUQuota = ref(20);
const nodePointsOverdraftMemoryGB = ref(8);
const nodePointsCPUControlEnabled = ref(true);
const nodeDiskQuotaVisible = ref(false);
const nodeDiskQuotaLoading = ref(false);
const nodeDiskQuotaSaving = ref(false);
const nodeDiskQuotaApplyingAll = ref(false);
const diskQuotaApplyingUserKey = ref("");
const nodeDiskQuotaNodeId = ref("");
const nodeDiskQuotaInstalled = ref(false);
const nodeDiskQuotaMounts = ref<string[]>([]);
const nodeDiskQuotaPreferredMountpoint = ref("");
const nodeDiskQuotaMountpoint = ref("");
const nodeDiskQuotaEnabled = ref(false);
const nodeDiskQuotaDefaultSoftGB = ref(20);
const nodeDiskQuotaDefaultHardGB = ref(22);
const nodeDiskQuotaApplyAllOnSave = ref(false);
type DiskQuotaUserRow = NodeLocalUser & { edit_soft_gb: number; edit_hard_gb: number };
const nodeDiskQuotaUsers = ref<DiskQuotaUserRow[]>([]);
const nodePriceVisible = ref(false);
const nodePriceSaving = ref(false);
const nodePriceNodeId = ref("");
const nodePriceCPUModel = ref("");
const nodePriceGPUModel = ref("");
const nodePricePerMinute = ref(0.1);
const nodeCPUPricePerCoreMinute = ref(0.02);
const nodePriceDefaultPerMinute = ref(0.1);
const nodeCPUPriceDefaultPerCoreMinute = ref(0.02);
const nodePriceRuleFormula = ref("每个进程总费用 = GPU费用 + CPU费用；最终按上报周期折算。");
const nodePriceRuleGPUPriority = ref<string[]>(["节点GPU单价", "全局GPU型号单价", "默认GPU单价"]);
const nodePriceRuleCPUPriority = ref<string[]>(["节点CPU单价", "全局CPU单价(CPU_CORE)", "默认CPU单价"]);
const detailVisible = ref(false);
const detailLoading = ref(false);
const detailError = ref("");
const detailNodeId = ref("");
const detailData = ref<NodeDetailResp | null>(null);
const securityEventsLoading = ref(false);
const securityRange = ref<string[]>([]);
const securityEventTypeFilter = ref("");
const securityShowSummary = ref(true);
const securityEventsRows = ref<NodeSecurityEvent[]>([]);
const securityEventSummariesRows = ref<NodeSecurityEventSummary[]>([]);
const securitySummaryNormalizer = ref("event_type + severity + reason(数字归一化)");
const lastRefreshAt = ref<number | null>(null);
const detailLastRefreshAt = ref<number | null>(null);
const disconnectingUserKey = ref("");
const blacklistingUserKey = ref("");
const sshOnlineRows = ref<Array<{
  local_username: string;
  platform_exists: boolean;
  platform_username: string;
  real_name: string;
  mapping_exists: boolean;
  admin_mapping: boolean;
  admin_username: string;
  message: string;
  loading: boolean;
}>>([]);
const platformProfileVisible = ref(false);
const platformProfileLoading = ref(false);
const platformProfileError = ref("");
const platformProfile = ref<{
  username: string;
  email: string;
  real_name: string;
  student_id: string;
  advisor: string;
  expected_graduation_year: number;
  expected_graduation_month: number;
  phone: string;
  role: string;
  balance: number;
  status: string;
  node_accounts: UserNodeAccount[];
} | null>(null);
let detailTimer: ReturnType<typeof setTimeout> | null = null;
let listTimer: ReturnType<typeof setTimeout> | null = null;
let sshResolveSeq = 0;
const DETAIL_AUTO_REFRESH_MS = 10 * 1000;
const LIST_AUTO_REFRESH_MS = 10 * 1000;

const totalGpuProcesses = computed(() => rows.value.reduce((sum, node) => sum + node.gpu_process_count, 0));
const totalCpuProcesses = computed(() => rows.value.reduce((sum, node) => sum + node.cpu_process_count, 0));
const totalCost = computed(() => rows.value.reduce((sum, node) => sum + node.cost_total, 0));
const onlineNodeCount = computed(() => rows.value.filter((node) => getNodeStatus(node) === "online").length);
const sortedRows = computed(() => [...rows.value].sort((a, b) => nodeIdSortValue(b.node_id) - nodeIdSortValue(a.node_id)));
const latestAgentVersion = computed(() => detectLatestAgentVersion(rows.value));
const riskyNodes = computed(() => {
  return [...rows.value]
    .filter((node) => hasNodeSecurityRisk(node))
    .sort((a, b) => {
      const e = Number(b.security_event_count_7d || 0) - Number(a.security_event_count_7d || 0);
      if (e !== 0) return e;
      const s = Number(b.suspicious_user_count_7d || 0) - Number(a.suspicious_user_count_7d || 0);
      if (s !== 0) return s;
      return nodeIdSortValue(b.node_id) - nodeIdSortValue(a.node_id);
    })
    .map((node) => {
      const eventCount = Number(node.security_event_count_7d || 0);
      const suspiciousCount = Number(node.suspicious_user_count_7d || 0);
      return {
        node_id: node.node_id,
        emoji: nodeRiskEmoji(node),
        summary: `事件 ${eventCount} / 疑似账号 ${suspiciousCount}`,
      };
    });
});
const history = computed(() => detailData.value?.history ?? []);
const cpuUtilNow = computed(() => calcCPUUtil(detailData.value?.latest));
const gpuUtilNow = computed(() => calcGPUUtil(detailData.value?.latest));
const lastRefreshTimeText = computed(() => (lastRefreshAt.value ? formatTime(lastRefreshAt.value) : "尚未刷新"));
const detailRefreshTimeText = computed(() => (detailLastRefreshAt.value ? formatTime(detailLastRefreshAt.value) : "-"));
const isSuperAdmin = computed(() => authState.role === "admin");
const nodeExclusiveGPUOptions = computed(() => {
  const n = Math.max(0, Number(nodeExclusiveGPUCount.value || 0));
  return Array.from({ length: n }, (_, idx) => idx);
});

function normalizeGPUIndexList(v: number[]): number[] {
  const set = new Set<number>();
  for (const idx of v || []) {
    const n = Number(idx);
    if (!Number.isInteger(n) || n < 0) continue;
    if (nodeExclusiveGPUCount.value > 0 && n >= nodeExclusiveGPUCount.value) continue;
    set.add(n);
  }
  return [...set].sort((a, b) => a - b);
}

function syncNodeExclusiveGPUAssignments() {
  const next: Record<string, number[]> = {};
  for (const u of nodeExclusiveUsers.value) {
    const name = String(u || "").trim();
    if (!name) continue;
    const current = nodeExclusiveGPUAssignments.value[name] || [];
    next[name] = normalizeGPUIndexList(current);
  }
  nodeExclusiveGPUAssignments.value = next;
}

watch(nodeExclusiveUsers, () => {
  syncNodeExclusiveGPUAssignments();
});

function calcCPUUtil(s?: NodeRuntimeSnapshot): number {
  if (!s || !detailData.value) return 0;
  const cpuCount = Number(detailData.value.node.cpu_count || 0);
  if (cpuCount <= 0) return 0;
  const v = s.cpu_percent_sum / cpuCount;
  if (!Number.isFinite(v) || v < 0) return 0;
  return Math.min(100, v);
}

function calcGPUUtil(s?: NodeRuntimeSnapshot): number {
  if (!s || !detailData.value) return 0;
  const gpuCount = Number(detailData.value.node.gpu_count || 0);
  if (gpuCount <= 0) return 0;
  const v = (s.gpu_process_count / gpuCount) * 100;
  if (!Number.isFinite(v) || v < 0) return 0;
  return Math.min(100, v);
}

function toLinePoints(values: number[]): string {
  if (!values.length) return "";
  const width = 520;
  const height = 130;
  const max = Math.max(...values, 1);
  if (values.length === 1) {
    const y = height - (values[0] / max) * (height - 4);
    return `0,${y} ${width},${y}`;
  }
  return values
    .map((v, i) => {
      const x = (i / (values.length - 1)) * width;
      const y = height - (v / max) * (height - 4);
      return `${x},${y}`;
    })
    .join(" ");
}

const cpuLinePoints = computed(() => toLinePoints(history.value.map((x) => calcCPUUtil(x))));
const memoryLinePoints = computed(() => toLinePoints(history.value.map((x) => Number(x.memory_mb_sum || 0))));
const sshLinePoints = computed(() => toLinePoints(history.value.map((x) => Number(x.ssh_user_count || 0))));
const gpuProcLinePoints = computed(() => toLinePoints(history.value.map((x) => Number(x.gpu_process_count || 0))));
const canManageNodes = computed(() => authState.role === "admin" || (authState.role === "power_user" && authState.canManageNodes));
const detailHasProtectedAdminMappings = computed(() => {
  if (authState.role !== "power_user") return false;
  return (detailData.value?.local_users || []).some((u) => !!u.admin_mapping);
});
const userLimitGPUOptions = computed(() => {
  const gpuCount = Number(detailData.value?.node?.gpu_count || 0);
  if (!Number.isFinite(gpuCount) || gpuCount <= 0) return [] as number[];
  return Array.from({ length: gpuCount }, (_, i) => i);
});

function formatTime(time: string | number | Date | null | undefined): string {
  if (!time) return "-";
  return formatServerDateTime(time);
}

function mappingStateTip(row: UserNodeAccount): string {
  if (row.identity_initializing) return "正在同步 UID/GID，完成前无法 SSH 登录";
  if (!row.identity_aligned) return "节点尚未回传最新 UID/GID 快照，请稍后自动刷新";
  return "";
}

function buildDefaultSecurityRange(): string[] {
  const now = Date.now();
  return [formatServerDateTime(now - 7 * 24 * 60 * 60 * 1000), formatServerDateTime(now)];
}

function currentSecurityRangeParams(): { from?: string; to?: string } {
  if (securityRange.value.length !== 2) return {};
  const from = String(securityRange.value[0] || "").trim();
  const to = String(securityRange.value[1] || "").trim();
  if (!from || !to) return {};
  return { from, to };
}

function applyNodePointsPolicyToRows(nodeId: string, policy: {
  enabled?: boolean;
  throttle_threshold_points?: number;
  limited_cpu_quota_percent?: number;
  blocked_cpu_quota_percent?: number;
  overdraft_memory_limit_gb?: number;
}) {
  const id = String(nodeId || "").trim();
  if (!id) return;
  const row = rows.value.find((x) => String(x.node_id || "").trim() === id);
  if (row) {
    if (typeof policy.enabled === "boolean") row.points_intercept_enabled = policy.enabled;
    if (typeof policy.throttle_threshold_points === "number") row.points_throttle_threshold = policy.throttle_threshold_points;
    if (typeof policy.limited_cpu_quota_percent === "number") row.points_limited_cpu_quota_percent = policy.limited_cpu_quota_percent;
    if (typeof policy.blocked_cpu_quota_percent === "number") row.points_blocked_cpu_quota_percent = policy.blocked_cpu_quota_percent;
    if (typeof policy.overdraft_memory_limit_gb === "number") row.points_overdraft_memory_limit_gb = policy.overdraft_memory_limit_gb;
  }
  if (detailData.value && String(detailData.value.node.node_id || "").trim() === id) {
    if (typeof policy.enabled === "boolean") detailData.value.node.points_intercept_enabled = policy.enabled;
    if (typeof policy.throttle_threshold_points === "number") detailData.value.node.points_throttle_threshold = policy.throttle_threshold_points;
    if (typeof policy.limited_cpu_quota_percent === "number") detailData.value.node.points_limited_cpu_quota_percent = policy.limited_cpu_quota_percent;
    if (typeof policy.blocked_cpu_quota_percent === "number") detailData.value.node.points_blocked_cpu_quota_percent = policy.blocked_cpu_quota_percent;
    if (typeof policy.overdraft_memory_limit_gb === "number") detailData.value.node.points_overdraft_memory_limit_gb = policy.overdraft_memory_limit_gb;
  }
}

function applyNodeDiskQuotaPolicyToRows(nodeId: string, policy: {
  enabled?: boolean;
  mountpoint?: string;
  default_soft_mb?: number;
  default_hard_mb?: number;
}) {
  const id = String(nodeId || "").trim();
  if (!id) return;
  const row = rows.value.find((x) => String(x.node_id || "").trim() === id);
  if (row) {
    if (typeof policy.enabled === "boolean") row.disk_quota_enabled = policy.enabled;
    if (typeof policy.mountpoint === "string") row.disk_quota_mountpoint = policy.mountpoint;
    if (typeof policy.default_soft_mb === "number") row.disk_quota_soft_mb = policy.default_soft_mb;
    if (typeof policy.default_hard_mb === "number") row.disk_quota_hard_mb = policy.default_hard_mb;
  }
  if (detailData.value && String(detailData.value.node.node_id || "").trim() === id) {
    if (typeof policy.enabled === "boolean") detailData.value.node.disk_quota_enabled = policy.enabled;
    if (typeof policy.mountpoint === "string") detailData.value.node.disk_quota_mountpoint = policy.mountpoint;
    if (typeof policy.default_soft_mb === "number") detailData.value.node.disk_quota_soft_mb = policy.default_soft_mb;
    if (typeof policy.default_hard_mb === "number") detailData.value.node.disk_quota_hard_mb = policy.default_hard_mb;
  }
}

function fmtGB(v?: number): string {
  const n = Number(v || 0);
  if (!Number.isFinite(n) || n <= 0) return "-";
  return `${n.toFixed(2)} GB`;
}

function formatGPUIndices(indices?: number[]): string {
  const arr = normalizeGPUIndexList((indices || []).map((x) => Number(x)));
  if (arr.length === 0) return "-";
  return arr.join(",");
}

function isProtectedAdminMapping(row: NodeLocalUser): boolean {
  return authState.role === "power_user" && !!row.admin_mapping;
}

function hasManualRestriction(row: NodeLocalUser): boolean {
  const cpu = typeof row.cpu_quota_percent === "number" ? Number(row.cpu_quota_percent) : 0;
  const memory = typeof row.memory_limit_gb === "number" ? Number(row.memory_limit_gb) : 0;
  const gpu = Array.isArray(row.gpu_visible_indices) ? row.gpu_visible_indices.length : 0;
  return cpu > 0 || memory > 0 || gpu > 0;
}

function openDetailUserLimitDialog(row: NodeLocalUser) {
  if (isProtectedAdminMapping(row)) {
    ElMessage.warning("高级用户不能操作管理员映射账号");
    return;
  }
  const nodeId = String(detailNodeId.value || "").trim();
  const local = String(row.local_username || "").trim();
  if (!nodeId || !local) return;
  userLimitNodeId.value = nodeId;
  userLimitLocalUsername.value = local;
  userLimitPlatformUsername.value = String(row.platform_username || "").trim();
  const cpuCurrent = typeof row.cpu_quota_percent === "number" ? Number(row.cpu_quota_percent) : 0;
  const memoryCurrent = typeof row.memory_limit_gb === "number" ? Number(row.memory_limit_gb) : 0;
  userLimitCPUEnabled.value = cpuCurrent > 0;
  userLimitCPUPercent.value = Number((cpuCurrent > 0 ? cpuCurrent : 50).toFixed(1));
  userLimitMemoryEnabled.value = memoryCurrent > 0;
  userLimitMemoryGB.value = Number((memoryCurrent > 0 ? memoryCurrent : 8).toFixed(1));
  const gpuCurrent = normalizeGPUIndexList((row.gpu_visible_indices || []).map((x) => Number(x)));
  userLimitGPUEnabled.value = gpuCurrent.length > 0;
  userLimitVisibleGPUIndices.value = [...gpuCurrent];
  userLimitReason.value = String(row.cpu_quota_reason || row.memory_limit_reason || row.gpu_visibility_reason || "").trim();
  userLimitDialogVisible.value = true;
}

async function deleteCPUUserLimitSafe(nodeId: string, local: string) {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  try {
    await client.adminDeleteNodeUserCPULimit(nodeId, local);
  } catch (e: any) {
    const status = Number(e?.status || 0);
    if (status !== 404) throw e;
  }
}

async function deleteMemoryUserLimitSafe(nodeId: string, local: string) {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  try {
    await client.adminDeleteNodeUserMemoryLimit(nodeId, local);
  } catch (e: any) {
    const status = Number(e?.status || 0);
    if (status !== 404) throw e;
  }
}

async function deleteGPUVisibilitySafe(nodeId: string, local: string) {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  try {
    await client.adminDeleteNodeUserGPUVisibility(nodeId, local);
  } catch (e: any) {
    const status = Number(e?.status || 0);
    if (status !== 404) throw e;
  }
}

const DISK_QUOTA_MB_PER_GB = 1024;
const DEFAULT_NODE_DISK_QUOTA_SOFT_GB = 20;
const DEFAULT_NODE_DISK_QUOTA_HARD_GB = 22;

function mbToQuotaGB(v?: number): number {
  const n = Number(v ?? 0);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Number((n / DISK_QUOTA_MB_PER_GB).toFixed(2));
}

function quotaGBToMB(v?: number): number {
  const n = Number(v ?? 0);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * DISK_QUOTA_MB_PER_GB);
}

function fmtQuotaGBFromMB(v?: number, zeroAsUnlimited = false): string {
  const gb = mbToQuotaGB(v);
  if (gb <= 0) return zeroAsUnlimited ? "0 (不限额)" : "0";
  return `${gb.toFixed(2)}`;
}

function diskUsagePercent(total?: number, used?: number): string {
  const t = Number(total || 0);
  const u = Number(used || 0);
  if (!Number.isFinite(t) || t <= 0 || !Number.isFinite(u) || u < 0) return "-";
  return `${Math.min(100, (u / t) * 100).toFixed(2)}%`;
}

function formatDiskUsage(total?: number, used?: number): string {
  const t = Number(total || 0);
  const u = Number(used || 0);
  if (!Number.isFinite(t) || t <= 0) return "-";
  const usedText = Number.isFinite(u) && u >= 0 ? u.toFixed(2) : "0.00";
  return `${t.toFixed(2)} / ${usedText} GB`;
}

function nodeIdSortValue(nodeID: string): number {
  const raw = String(nodeID || "").trim();
  const m = raw.match(/\d+/g);
  if (!m || m.length === 0) return Number.NEGATIVE_INFINITY;
  const n = Number(m[m.length - 1]);
  if (!Number.isFinite(n)) return Number.NEGATIVE_INFINITY;
  return n;
}

function normalizeAgentVersion(v: unknown): string {
  return String(v ?? "").trim();
}

const AGENT_VERSION_PATTERN = /^v?(\d+)\.(\d+)(?:\.\d+)?(?:[-+].*)?$/i;

function parseAgentMajorMinor(v: unknown): [number, number] | null {
  const m = normalizeAgentVersion(v).match(AGENT_VERSION_PATTERN);
  if (!m) return null;
  const major = Number(m[1]);
  const minor = Number(m[2]);
  if (!Number.isInteger(major) || major < 0) return null;
  if (!Number.isInteger(minor) || minor < 0) return null;
  return [major, minor];
}

function formatAgentMajorMinor(v: [number, number]): string {
  return `v${v[0]}.${v[1]}`;
}

function displayAgentVersion(v: unknown): string {
  const raw = normalizeAgentVersion(v);
  if (!raw) return "-";
  return raw;
}

function compareAgentMajorMinor(a: [number, number], b: [number, number]): number {
  if (a[0] !== b[0]) return a[0] - b[0];
  return a[1] - b[1];
}

function detectLatestAgentVersion(nodes: NodeStatus[]): string {
  let latest: [number, number] | null = null;
  for (const node of nodes || []) {
    const parsed = parseAgentMajorMinor(node.agent_version);
    if (!parsed) continue;
    if (!latest || compareAgentMajorMinor(parsed, latest) > 0) {
      latest = parsed;
    }
  }
  return latest ? formatAgentMajorMinor(latest) : "";
}

function isAgentVersionOutdated(node: NodeStatus): boolean {
  const latest = parseAgentMajorMinor(latestAgentVersion.value);
  if (!latest) return false;
  const current = parseAgentMajorMinor(node.agent_version);
  if (!current) return true;
  return compareAgentMajorMinor(current, latest) < 0;
}

function agentVersionOutdatedTip(node: NodeStatus): string {
  const latest = parseAgentMajorMinor(latestAgentVersion.value);
  const latestText = latest ? formatAgentMajorMinor(latest) : "-";
  const current = parseAgentMajorMinor(node.agent_version);
  if (!current) {
    const raw = normalizeAgentVersion(node.agent_version);
    return `该节点 Agent 版本为未知格式（${raw || "未上报"}），当前集群最新为 ${latestText}；判定依据为主次版本（vX.Y）。`;
  }
  return `该节点 Agent 版本为 ${formatAgentMajorMinor(current)}，当前集群最新为 ${latestText}；判定依据为主次版本（vX.Y）。`;
}

function latestNodeHeartbeat(node: NodeStatus) {
  const candidates = [
    dayjs(String(node?.last_seen_at || "")),
    dayjs(String(node?.last_report_ts || "")),
    dayjs(String(node?.updated_at || "")),
  ].filter((t) => t.isValid());
  if (candidates.length === 0) return null;
  return candidates.sort((a, b) => a.valueOf() - b.valueOf())[candidates.length - 1];
}

function nodeHeartbeatTimeoutSeconds(node: NodeStatus): number {
  const interval = Number(node?.interval_seconds || 0);
  const scaled = Number.isFinite(interval) && interval > 0 ? interval * 3 : 0;
  return Math.max(5 * 60, scaled);
}

function getNodeStatus(node: NodeStatus): "online" | "offline" {
  const latest = latestNodeHeartbeat(node);
  if (!latest) return "offline";
  return dayjs().diff(latest, "second") <= nodeHeartbeatTimeoutSeconds(node) ? "online" : "offline";
}

function nodeStatusText(node: NodeStatus): string {
  const st = getNodeStatus(node);
  if (st === "online") return "上报在线";
  return "上报超时";
}

function nodeStatusTagType(node: NodeStatus): "success" | "info" | "warning" {
  const st = getNodeStatus(node);
  if (st === "online") return "success";
  return "warning";
}

function canClickNodeHeartbeatTag(node: NodeStatus): boolean {
  return getNodeStatus(node) === "online" && syncingNodeId.value !== String(node.node_id || "").trim();
}

type NodeServiceHealthState = "healthy" | "degraded" | "stale" | "unknown" | "not_deployed";

function normalizeSystemServices(node: NodeStatus): NodeSystemServiceStatus[] {
  return (node.system_services || []).filter((item) => String(item?.name || "").trim() !== "");
}

function deployedSystemServices(node: NodeStatus): NodeSystemServiceStatus[] {
  return normalizeSystemServices(node).filter((item) => !!item.deployed);
}

function unhealthySystemServices(node: NodeStatus): NodeSystemServiceStatus[] {
  return deployedSystemServices(node).filter((item) => !item.healthy);
}

function canFallbackNodeServiceHealthy(node: NodeStatus): boolean {
  return getNodeStatus(node) === "online";
}

function nodeServiceHealthState(node: NodeStatus): NodeServiceHealthState {
  const services = normalizeSystemServices(node);
  if (services.length === 0) {
    if (canFallbackNodeServiceHealthy(node)) return "healthy";
    return "unknown";
  }
  const deployed = deployedSystemServices(node);
  if (deployed.length === 0) return "not_deployed";
  const checkedAt = dayjs(String(node.system_services_checked_at || ""));
  if (!checkedAt.isValid()) return "unknown";
  if (dayjs().diff(checkedAt, "minute") > 45) return "stale";
  if (unhealthySystemServices(node).length > 0) return "degraded";
  return "healthy";
}

function nodeServiceHealthText(node: NodeStatus): string {
  const state = nodeServiceHealthState(node);
  const deployed = deployedSystemServices(node);
  const unhealthy = unhealthySystemServices(node);
  if (state === "healthy") return "服务正常";
  if (state === "degraded") return `服务异常 ${unhealthy.length}/${deployed.length}`;
  if (state === "stale") return "服务陈旧";
  if (state === "not_deployed") return "未部署服务";
  return "服务未检";
}

function nodeServiceHealthTagType(node: NodeStatus): "success" | "info" | "warning" | "danger" {
  const state = nodeServiceHealthState(node);
  if (state === "healthy") return "success";
  if (state === "degraded") return "danger";
  if (state === "stale") return "warning";
  return "info";
}

function canClickNodeServiceTag(node: NodeStatus): boolean {
  return nodeServiceHealthState(node) === "unknown" && syncingNodeId.value !== String(node.node_id || "").trim();
}

function displaySystemServiceName(name: string): string {
  const raw = String(name || "").trim();
  if (!raw) return "-";
  return raw.replace(/\.service$/i, "").replace(/\.timer$/i, ".timer");
}

function displaySystemServiceState(item: NodeSystemServiceStatus): string {
  if (!item.deployed) return "未部署";
  const active = String(item.active_state || "").trim();
  const sub = String(item.sub_state || "").trim();
  if (active === "active") return sub ? `运行中/${sub}` : "运行中";
  if (active === "inactive") return sub ? `已停/${sub}` : "已停";
  if (active === "failed") return sub ? `失败/${sub}` : "失败";
  if (active === "activating") return "启动中";
  if (active === "deactivating") return "停止中";
  return active || sub || "未知";
}

function nodeServiceHealthTooltipTitle(node: NodeStatus): string {
  if (normalizeSystemServices(node).length === 0 && canFallbackNodeServiceHealthy(node)) {
    return "服务巡检：未单独上报，按在线 Agent 兜底判定";
  }
  const checkedAt = formatTime(node.system_services_checked_at);
  return `服务巡检：${checkedAt}`;
}

function nodeServiceHealthTooltipLines(node: NodeStatus): string[] {
  const services = normalizeSystemServices(node);
  if (services.length === 0) {
    if (canFallbackNodeServiceHealthy(node)) {
      return [
        `系统版本: ${String(node.os_version || "-").trim() || "-"}`,
        `内核版本: ${String(node.kernel_version || "-").trim() || "-"}`,
        `Agent版本: ${displayAgentVersion(node.agent_version)}`,
        "当前节点持续在线，按 gpu-node-agent 正在运行处理",
      ];
    }
    return ["暂无服务巡检数据"];
  }
  return services.map((item) => `${displaySystemServiceName(item.name)}: ${displaySystemServiceState(item)}`);
}

async function onNodeHeartbeatTagClick(node: NodeStatus) {
  if (!canClickNodeHeartbeatTag(node)) return;
  await syncNodeNow(String(node.node_id || "").trim());
}

async function onNodeServiceTagClick(node: NodeStatus) {
  if (!canClickNodeServiceTag(node)) return;
  await syncNodeNow(String(node.node_id || "").trim());
}

function hasNodeSecurityEvents(node: NodeStatus): boolean {
  return Number(node.security_event_count_7d || 0) > 0;
}

function hasNodeSuspiciousUsers(node: NodeStatus): boolean {
  return Number(node.suspicious_user_count_7d || 0) > 0;
}

function hasNodeSecurityRisk(node: NodeStatus): boolean {
  return hasNodeSecurityEvents(node) || hasNodeSuspiciousUsers(node);
}

function nodeRiskEmoji(node: NodeStatus): string {
  if (hasNodeSecurityEvents(node)) return "🚨";
  if (hasNodeSuspiciousUsers(node)) return "⚠️";
  return "";
}

function nodeRiskTooltip(node: NodeStatus): string {
  return `近7天安全事件 ${Number(node.security_event_count_7d || 0)} 条；疑似账号 ${Number(node.suspicious_user_count_7d || 0)} 个`;
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodes(200);
    rows.value = r.nodes ?? [];
    lastRefreshAt.value = Date.now();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

function applyNodeStatusSnapshot(node: NodeStatus | null | undefined) {
  const id = String(node?.node_id || "").trim();
  if (!id || !node) return;
  const idx = rows.value.findIndex((x) => String(x.node_id || "").trim() === id);
  if (idx < 0) return;
  const next = [...rows.value];
  next[idx] = { ...next[idx], ...node };
  rows.value = next;
}

async function onNodeSSHGuardToggle(row: NodeStatus, enabled: boolean) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  const prev = !!row.ssh_guard_enabled;
  if (enabled === prev) return;
  if (enabled) {
    try {
      await ElMessageBox.confirm(
        `开启后将拦截平台未注册用户 SSH 登录，并立即清除节点 ${nodeId} 的全部 SSH 会话，所有用户需要重新登录。是否继续？`,
        "二次确认",
        { type: "warning", confirmButtonText: "确认开启", cancelButtonText: "取消" },
      );
    } catch {
      row.ssh_guard_enabled = prev;
      return;
    }
  }
  guardUpdatingNodeId.value = nodeId;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminSetNodeSSHGuard(nodeId, enabled);
    row.ssh_guard_enabled = !!r.enabled;
    if (enabled) {
      ElMessage.success(r.kick_triggered ? `节点 ${nodeId} 已开启未注册拦截，SSH 会话已清空` : `节点 ${nodeId} 已开启未注册拦截`);
    } else {
      ElMessage.success(`节点 ${nodeId} 已关闭未注册拦截（当前会话不受影响）`);
    }
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    row.ssh_guard_enabled = prev;
    ElMessage.error(e?.message ?? String(e));
  } finally {
    guardUpdatingNodeId.value = "";
  }
}

async function onNodePointsInterceptToggle(row: NodeStatus, enabled: boolean) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  const prev = !!row.points_intercept_enabled;
  if (enabled === prev) return;
  pointsUpdatingNodeId.value = nodeId;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminSetNodePointsIntercept(nodeId, enabled);
    let syncErr = "";
    try {
      await client.adminSyncNodeNow(nodeId);
    } catch (e: any) {
      syncErr = e?.message ?? String(e);
    }
    applyNodePointsPolicyToRows(nodeId, {
      enabled: !!r.enabled,
      throttle_threshold_points: Number(r.throttle_threshold_points ?? row.points_throttle_threshold ?? 0),
      limited_cpu_quota_percent: Number(r.limited_cpu_quota_percent ?? row.points_limited_cpu_quota_percent ?? 0),
      blocked_cpu_quota_percent: Number(r.blocked_cpu_quota_percent ?? row.points_blocked_cpu_quota_percent ?? 0),
    });
    if (syncErr) {
      ElMessage.success(r.enabled ? `节点 ${nodeId} 已开启正常扣分与限速（将按轮询自动生效）` : `节点 ${nodeId} 已关闭扣分与限速（仅记录使用）`);
      ElMessage.warning(`节点 ${nodeId} 立即同步下发失败：${syncErr}`);
    } else {
      ElMessage.success(r.enabled ? `节点 ${nodeId} 已开启正常扣分与限速，并已实时同步` : `节点 ${nodeId} 已关闭扣分与限速（仅记录使用），并已实时同步`);
      await wait(1200);
      await reload();
    }
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    row.points_intercept_enabled = prev;
    ElMessage.error(e?.message ?? String(e));
  } finally {
    pointsUpdatingNodeId.value = "";
  }
}

async function onNodeDiskQuotaToggle(row: NodeStatus, enabled: boolean) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  const prev = !!row.disk_quota_enabled;
  if (enabled === prev) return;
  diskQuotaUpdatingNodeId.value = nodeId;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminSetNodeDiskQuota(nodeId, { enabled });
    let syncErr = "";
    try {
      await client.adminSyncNodeNow(nodeId);
    } catch (e: any) {
      syncErr = e?.message ?? String(e);
    }
    applyNodeDiskQuotaPolicyToRows(nodeId, {
      enabled: !!r.enabled,
      mountpoint: String(r.mountpoint || ""),
      default_soft_mb: Number(r.default_soft_mb ?? row.disk_quota_soft_mb ?? 0),
      default_hard_mb: Number(r.default_hard_mb ?? row.disk_quota_hard_mb ?? 0),
    });
    if (r.warning) {
      ElMessage.warning(String(r.warning));
    }
    if (syncErr) {
      ElMessage.success(`节点 ${nodeId} 硬盘配额策略已更新（将按轮询自动生效）`);
      ElMessage.warning(`节点 ${nodeId} 立即同步下发失败：${syncErr}`);
    } else {
      ElMessage.success(`节点 ${nodeId} 硬盘配额策略已更新，并已实时同步`);
      await wait(1200);
      await reload();
    }
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    row.disk_quota_enabled = prev;
    ElMessage.error(e?.message ?? String(e));
  } finally {
    diskQuotaUpdatingNodeId.value = "";
  }
}

function toDiskQuotaUserRows(users: NodeLocalUser[], fallbackSoftGB: number, fallbackHardGB: number): DiskQuotaUserRow[] {
  return (users || []).map((u) => {
    const softGB = u.quota_soft_mb != null ? mbToQuotaGB(u.quota_soft_mb) : Number(fallbackSoftGB || 0);
    const hardGB = u.quota_hard_mb != null ? mbToQuotaGB(u.quota_hard_mb) : Number(fallbackHardGB || 0);
    return {
      ...u,
      edit_soft_gb: Number.isFinite(softGB) ? softGB : 0,
      edit_hard_gb: Number.isFinite(hardGB) ? hardGB : 0,
    };
  });
}

async function loadNodeDiskQuotaDialog(nodeId: string, withLoading = true) {
  const id = String(nodeId || "").trim();
  if (!id) return;
  if (withLoading) nodeDiskQuotaLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodeDiskQuota(id);
    nodeDiskQuotaInstalled.value = !!r.quota_installed;
    nodeDiskQuotaMounts.value = Array.isArray(r.quota_mounts) ? r.quota_mounts : [];
    nodeDiskQuotaPreferredMountpoint.value = String(r.preferred_mountpoint || "");
    nodeDiskQuotaEnabled.value = !!r.enabled;
    nodeDiskQuotaMountpoint.value = String(r.mountpoint || r.effective_mountpoint || nodeDiskQuotaPreferredMountpoint.value || "");
    nodeDiskQuotaDefaultSoftGB.value = mbToQuotaGB(Number(r.effective_soft_mb ?? r.default_soft_mb ?? quotaGBToMB(DEFAULT_NODE_DISK_QUOTA_SOFT_GB)));
    nodeDiskQuotaDefaultHardGB.value = mbToQuotaGB(Number(r.effective_hard_mb ?? r.default_hard_mb ?? quotaGBToMB(DEFAULT_NODE_DISK_QUOTA_HARD_GB)));
    nodeDiskQuotaUsers.value = toDiskQuotaUserRows(
      Array.isArray(r.users) ? r.users : [],
      nodeDiskQuotaDefaultSoftGB.value,
      nodeDiskQuotaDefaultHardGB.value,
    );
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
    throw e;
  } finally {
    if (withLoading) nodeDiskQuotaLoading.value = false;
  }
}

async function openNodeDiskQuotaDialog(row: NodeStatus) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  nodeDiskQuotaNodeId.value = nodeId;
  nodeDiskQuotaApplyAllOnSave.value = false;
  nodeDiskQuotaVisible.value = true;
  nodeDiskQuotaSaving.value = false;
  nodeDiskQuotaApplyingAll.value = false;
  diskQuotaApplyingUserKey.value = "";
  try {
    await loadNodeDiskQuotaDialog(nodeId, true);
  } catch {
    nodeDiskQuotaVisible.value = false;
  }
}

function validateDiskQuotaInput(softGB: number, hardGB: number): string {
  if (!Number.isFinite(softGB) || softGB < 0) return "软配额（G）必须是非负数";
  if (!Number.isFinite(hardGB) || hardGB < 0) return "硬配额（G）必须是非负数";
  if (softGB > 0 && hardGB > 0 && hardGB < softGB) return "硬配额（G）不能小于软配额（G）";
  return "";
}

async function saveNodeDiskQuotaPolicy() {
  const nodeId = String(nodeDiskQuotaNodeId.value || "").trim();
  if (!nodeId) return;
  const errMsg = validateDiskQuotaInput(Number(nodeDiskQuotaDefaultSoftGB.value), Number(nodeDiskQuotaDefaultHardGB.value));
  if (errMsg) {
    ElMessage.error(errMsg);
    return;
  }
  if (!String(nodeDiskQuotaMountpoint.value || "").trim() && nodeDiskQuotaEnabled.value) {
    ElMessage.error("请先选择配额分区（/home 或 /）");
    return;
  }
  nodeDiskQuotaSaving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminSetNodeDiskQuota(nodeId, {
      enabled: !!nodeDiskQuotaEnabled.value,
      mountpoint: String(nodeDiskQuotaMountpoint.value || "").trim(),
      default_soft_mb: quotaGBToMB(Number(nodeDiskQuotaDefaultSoftGB.value)),
      default_hard_mb: quotaGBToMB(Number(nodeDiskQuotaDefaultHardGB.value)),
      apply_to_all: !!nodeDiskQuotaApplyAllOnSave.value,
    });
    let syncErr = "";
    try {
      await client.adminSyncNodeNow(nodeId);
    } catch (e: any) {
      syncErr = e?.message ?? String(e);
    }
    applyNodeDiskQuotaPolicyToRows(nodeId, {
      enabled: !!r.enabled,
      mountpoint: String(r.mountpoint || nodeDiskQuotaMountpoint.value || ""),
      default_soft_mb: Number(r.default_soft_mb ?? quotaGBToMB(nodeDiskQuotaDefaultSoftGB.value)),
      default_hard_mb: Number(r.default_hard_mb ?? quotaGBToMB(nodeDiskQuotaDefaultHardGB.value)),
    });
    if (r.warning) {
      ElMessage.warning(String(r.warning));
    }
    if (syncErr) {
      ElMessage.success("节点硬盘配额策略已保存（将按轮询自动生效）");
      ElMessage.warning(`节点 ${nodeId} 立即同步下发失败：${syncErr}`);
    } else {
      const applied = Number(r.applied_users || 0);
      ElMessage.success(applied > 0 ? `节点硬盘配额策略已保存，并已实时同步（已应用 ${applied} 个用户）` : "节点硬盘配额策略已保存，并已实时同步");
      await wait(1200);
    }
    await reload();
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
    await loadNodeDiskQuotaDialog(nodeId, false);
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    nodeDiskQuotaSaving.value = false;
  }
}

async function applyNodeDiskQuotaAll() {
  const nodeId = String(nodeDiskQuotaNodeId.value || "").trim();
  if (!nodeId) return;
  const errMsg = validateDiskQuotaInput(Number(nodeDiskQuotaDefaultSoftGB.value), Number(nodeDiskQuotaDefaultHardGB.value));
  if (errMsg) {
    ElMessage.error(errMsg);
    return;
  }
  if (!String(nodeDiskQuotaMountpoint.value || "").trim()) {
    ElMessage.error("请先选择配额分区");
    return;
  }
  nodeDiskQuotaApplyingAll.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminApplyNodeDiskQuota(nodeId, {
      all_users: true,
      mountpoint: String(nodeDiskQuotaMountpoint.value || "").trim(),
      soft_mb: quotaGBToMB(Number(nodeDiskQuotaDefaultSoftGB.value)),
      hard_mb: quotaGBToMB(Number(nodeDiskQuotaDefaultHardGB.value)),
    });
    let syncErr = "";
    try {
      await client.adminSyncNodeNow(nodeId);
    } catch (e: any) {
      syncErr = e?.message ?? String(e);
    }
    if (syncErr) {
      ElMessage.success(`已下发全体配额（${Number(r.applied_users || 0)} 个用户），将按轮询自动生效`);
      ElMessage.warning(`节点 ${nodeId} 立即同步下发失败：${syncErr}`);
    } else {
      ElMessage.success(`已下发全体配额（${Number(r.applied_users || 0)} 个用户）并实时同步`);
      await wait(1200);
    }
    await loadNodeDiskQuotaDialog(nodeId, false);
    await reload();
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    nodeDiskQuotaApplyingAll.value = false;
  }
}

async function applyNodeDiskQuotaUser(row: DiskQuotaUserRow) {
  const nodeId = String(nodeDiskQuotaNodeId.value || "").trim();
  const local = String(row.local_username || "").trim();
  if (!nodeId || !local) return;
  const softGB = Number(row.edit_soft_gb || 0);
  const hardGB = Number(row.edit_hard_gb || 0);
  const errMsg = validateDiskQuotaInput(softGB, hardGB);
  if (errMsg) {
    ElMessage.error(errMsg);
    return;
  }
  if (!String(nodeDiskQuotaMountpoint.value || "").trim()) {
    ElMessage.error("请先选择配额分区");
    return;
  }
  diskQuotaApplyingUserKey.value = `${nodeId}::${local}`;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminApplyNodeDiskQuota(nodeId, {
      mountpoint: String(nodeDiskQuotaMountpoint.value || "").trim(),
      users: [{ local_username: local, soft_mb: quotaGBToMB(softGB), hard_mb: quotaGBToMB(hardGB) }],
    });
    let syncErr = "";
    try {
      await client.adminSyncNodeNow(nodeId);
    } catch (e: any) {
      syncErr = e?.message ?? String(e);
    }
    if (syncErr) {
      ElMessage.success(`用户 ${local} 配额已下发（将按轮询自动生效）`);
      ElMessage.warning(`节点 ${nodeId} 立即同步下发失败：${syncErr}`);
    } else {
      ElMessage.success(`用户 ${local} 配额已下发并实时同步`);
      await wait(1200);
    }
    await loadNodeDiskQuotaDialog(nodeId, false);
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    diskQuotaApplyingUserKey.value = "";
  }
}

async function openNodeVisibilityDialog(row: NodeStatus) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  nodeVisibilityNodeId.value = nodeId;
  nodeVisibilityVisible.value = true;
  nodeVisibilitySaving.value = false;
  nodeVisibilityRestricted.value = false;
  nodeVisibilityAllowedUsers.value = [];
  nodeVisibilityCandidates.value = [];
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodeViewAccess(nodeId);
    nodeVisibilityRestricted.value = !!r.restricted;
    nodeVisibilityAllowedUsers.value = Array.isArray(r.allowed_power_users) ? [...r.allowed_power_users] : [];
    nodeVisibilityCandidates.value = Array.isArray(r.candidates) ? [...r.candidates] : [];
  } catch (e: any) {
    if (e?.status === 404) {
      ElMessage.error("节点可见性接口不可用：请确认控制器已更新到最新版本并重启");
    } else {
      ElMessage.error(e?.message ?? String(e));
    }
    nodeVisibilityVisible.value = false;
  }
}

async function saveNodeVisibility() {
  const nodeId = String(nodeVisibilityNodeId.value || "").trim();
  if (!nodeId) return;
  nodeVisibilitySaving.value = true;
  try {
    const payload = {
      restricted: !!nodeVisibilityRestricted.value,
      allowed_power_users: nodeVisibilityRestricted.value ? nodeVisibilityAllowedUsers.value : [],
    };
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminSetNodeViewAccess(nodeId, payload);
    ElMessage.success("节点可见性保存成功");
    nodeVisibilityVisible.value = false;
    await reload();
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    nodeVisibilitySaving.value = false;
  }
}

async function openNodeExclusiveDialog(row: NodeStatus) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  nodeExclusiveNodeId.value = nodeId;
  nodeExclusiveVisible.value = true;
  nodeExclusiveSaving.value = false;
  nodeExclusiveEnabled.value = false;
  nodeExclusiveBlockOtherSSH.value = true;
  nodeExclusiveUsers.value = [];
  nodeExclusiveCandidates.value = [];
  nodeExclusiveGPUCount.value = Number(row.gpu_count || 0);
  nodeExclusiveGPUAssignments.value = {};
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodeSSHExclusive(nodeId);
    nodeExclusiveEnabled.value = !!r.enabled;
    nodeExclusiveBlockOtherSSH.value = r.block_other_ssh !== false;
    nodeExclusiveUsers.value = Array.isArray(r.exclusive_users) ? [...r.exclusive_users] : [];
    nodeExclusiveCandidates.value = Array.isArray(r.candidate_local_users) ? [...r.candidate_local_users] : [];
    nodeExclusiveGPUCount.value = Number(r.gpu_count || row.gpu_count || 0);
    const m: Record<string, number[]> = {};
    for (const item of r.gpu_assignments || []) {
      const u = String(item.local_username || "").trim();
      if (!u) continue;
      m[u] = normalizeGPUIndexList((item.gpu_indices || []).map((x) => Number(x)));
    }
    nodeExclusiveGPUAssignments.value = m;
    syncNodeExclusiveGPUAssignments();
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
    nodeExclusiveVisible.value = false;
  }
}

async function saveNodeExclusive() {
  const nodeId = String(nodeExclusiveNodeId.value || "").trim();
  if (!nodeId) return;
  const users = [...new Set(nodeExclusiveUsers.value.map((x) => String(x || "").trim()).filter(Boolean))];
  const assignments = users.map((u) => ({
    local_username: u,
    gpu_indices: normalizeGPUIndexList(nodeExclusiveGPUAssignments.value[u] || []),
  }));
  nodeExclusiveSaving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const resp = await client.adminSetNodeSSHExclusive(nodeId, {
      enabled: !!nodeExclusiveEnabled.value,
      block_other_ssh: !!nodeExclusiveBlockOtherSSH.value,
      exclusive_users: nodeExclusiveEnabled.value ? users : [],
      gpu_assignments: nodeExclusiveEnabled.value ? assignments : [],
    });
    if ((resp.exempt_ignored_users || []).length > 0) {
      ElMessage.warning(`节点独享策略已保存，但豁免用户无视规则：${(resp.exempt_ignored_users || []).join(", ")}`);
    } else {
      ElMessage.success("节点独享策略保存成功，SSH/GPU 策略将立即同步");
    }
    nodeExclusiveVisible.value = false;
    await reload();
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    nodeExclusiveSaving.value = false;
  }
}

async function openNodePointsPolicyDialog(row: NodeStatus) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  nodePointsPolicyNodeId.value = nodeId;
  nodePointsPolicyVisible.value = true;
  nodePointsPolicySaving.value = false;
  nodePointsPolicyEnabled.value = !!row.points_intercept_enabled;
  nodePointsThrottleThreshold.value = Number(row.points_throttle_threshold ?? 0);
  nodePointsLimitedCPUQuota.value = Number(row.points_limited_cpu_quota_percent ?? 40);
  nodePointsBlockedCPUQuota.value = Number(row.points_blocked_cpu_quota_percent ?? 20);
  nodePointsOverdraftMemoryGB.value = Number(row.points_overdraft_memory_limit_gb ?? 8);
  nodePointsCPUControlEnabled.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodePointsIntercept(nodeId);
    nodePointsPolicyEnabled.value = !!r.enabled;
    nodePointsThrottleThreshold.value = Number(r.effective_threshold_points ?? r.throttle_threshold_points ?? 0);
    nodePointsLimitedCPUQuota.value = Number(r.effective_limited_cpu_quota ?? r.limited_cpu_quota_percent ?? 40);
    nodePointsBlockedCPUQuota.value = Number(r.effective_blocked_cpu_quota ?? r.blocked_cpu_quota_percent ?? 20);
    nodePointsOverdraftMemoryGB.value = Number(
      r.effective_overdraft_memory_gb ?? r.overdraft_memory_limit_gb ?? r.default_overdraft_memory_gb ?? 8,
    );
    nodePointsCPUControlEnabled.value = r.cpu_control_enabled_on_server !== false;
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
    nodePointsPolicyVisible.value = false;
  }
}

async function saveNodePointsPolicy() {
  const nodeId = String(nodePointsPolicyNodeId.value || "").trim();
  if (!nodeId) return;
  if (!Number.isFinite(nodePointsThrottleThreshold.value) || nodePointsThrottleThreshold.value < 0) {
    ElMessage.error("低积分限速阈值必须是非负数");
    return;
  }
  if (!Number.isFinite(nodePointsLimitedCPUQuota.value) || nodePointsLimitedCPUQuota.value < 1 || nodePointsLimitedCPUQuota.value > 100) {
    ElMessage.error("低积分限速比例必须在 1~100 之间");
    return;
  }
  if (!Number.isFinite(nodePointsBlockedCPUQuota.value) || nodePointsBlockedCPUQuota.value < 1 || nodePointsBlockedCPUQuota.value > 100) {
    ElMessage.error("欠费强限速比例必须在 1~100 之间");
    return;
  }
  if (!Number.isFinite(nodePointsOverdraftMemoryGB.value) || nodePointsOverdraftMemoryGB.value < 0) {
    ElMessage.error("欠费内存上限必须是非负数（0 表示关闭）");
    return;
  }
  nodePointsPolicySaving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminSetNodePointsIntercept(nodeId, {
      enabled: !!nodePointsPolicyEnabled.value,
      throttle_threshold_points: Number(nodePointsThrottleThreshold.value),
      limited_cpu_quota_percent: Number(nodePointsLimitedCPUQuota.value),
      blocked_cpu_quota_percent: Number(nodePointsBlockedCPUQuota.value),
      overdraft_memory_limit_gb: Number(nodePointsOverdraftMemoryGB.value),
    });
    let syncErr = "";
    try {
      await client.adminSyncNodeNow(nodeId);
    } catch (e: any) {
      syncErr = e?.message ?? String(e);
    }
    applyNodePointsPolicyToRows(nodeId, {
      enabled: !!r.enabled,
      throttle_threshold_points: Number(r.throttle_threshold_points ?? nodePointsThrottleThreshold.value),
      limited_cpu_quota_percent: Number(r.limited_cpu_quota_percent ?? nodePointsLimitedCPUQuota.value),
      blocked_cpu_quota_percent: Number(r.blocked_cpu_quota_percent ?? nodePointsBlockedCPUQuota.value),
      overdraft_memory_limit_gb: Number(r.overdraft_memory_limit_gb ?? nodePointsOverdraftMemoryGB.value),
    });
    nodePointsPolicyVisible.value = false;
    if (syncErr) {
      ElMessage.success("节点积分限速策略已保存（将按轮询自动生效）");
      ElMessage.warning(`节点 ${nodeId} 立即同步下发失败：${syncErr}`);
    } else {
      ElMessage.success("节点积分限速策略已保存，并已实时同步到计算节点");
      await wait(1200);
    }
    await reload();
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    nodePointsPolicySaving.value = false;
  }
}

async function openNodePriceDialog(row: NodeStatus) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  nodePriceNodeId.value = nodeId;
  nodePriceCPUModel.value = String(row.cpu_model || "").trim();
  nodePriceGPUModel.value = String(row.gpu_model || "").trim();
  nodePriceVisible.value = true;
  nodePriceSaving.value = false;
  nodePricePerMinute.value = Number(row.node_price_per_minute ?? 0.1);
  nodeCPUPricePerCoreMinute.value = 0.02;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodePrice(nodeId);
    nodePriceDefaultPerMinute.value = Number(r.default_price_per_minute ?? 0.1);
    nodeCPUPriceDefaultPerCoreMinute.value = Number(r.default_cpu_price_per_core_minute ?? 0.02);
    nodePricePerMinute.value = Number(r.price_per_minute ?? r.default_price_per_minute ?? 0.1);
    nodeCPUPricePerCoreMinute.value = Number(r.cpu_price_per_core_minute ?? r.default_cpu_price_per_core_minute ?? 0.02);
    nodePriceRuleFormula.value = String(
      r.billing_rules?.combined_formula || "每个进程总费用 = GPU费用 + CPU费用；最终按上报周期折算。",
    );
    nodePriceRuleGPUPriority.value = Array.isArray(r.billing_rules?.gpu_price_priority)
      ? (r.billing_rules?.gpu_price_priority ?? [])
      : ["节点GPU单价", "全局GPU型号单价", "默认GPU单价"];
    nodePriceRuleCPUPriority.value = Array.isArray(r.billing_rules?.cpu_price_priority)
      ? (r.billing_rules?.cpu_price_priority ?? [])
      : ["节点CPU单价", "全局CPU单价(CPU_CORE)", "默认CPU单价"];
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
    nodePriceVisible.value = false;
  }
}

async function saveNodePrice() {
  const nodeId = String(nodePriceNodeId.value || "").trim();
  if (!nodeId) return;
  if (!Number.isFinite(nodePricePerMinute.value) || Number(nodePricePerMinute.value) < 0) {
    ElMessage.error("请填写合法的节点 GPU 单价（>= 0）");
    return;
  }
  if (!Number.isFinite(nodeCPUPricePerCoreMinute.value) || Number(nodeCPUPricePerCoreMinute.value) < 0) {
    ElMessage.error("请填写合法的节点 CPU 单价（>= 0）");
    return;
  }
  nodePriceSaving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminSetNodePrice(nodeId, {
      price_per_minute: Number(nodePricePerMinute.value),
      cpu_price_per_core_minute: Number(nodeCPUPricePerCoreMinute.value),
    });
    ElMessage.success("节点计费参数已更新");
    nodePriceVisible.value = false;
    await reload();
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    nodePriceSaving.value = false;
  }
}

async function refreshSSHUserMappings(nodeId: string, users: string[]) {
  const uniq = Array.from(new Set((users ?? []).map((x) => String(x || "").trim()).filter(Boolean)));
  if (!uniq.length) {
    sshOnlineRows.value = [];
    return;
  }
  const seq = ++sshResolveSeq;
  const rows = uniq.map((u) => ({
    local_username: u,
    platform_exists: false,
    platform_username: "",
    real_name: "",
    mapping_exists: false,
    admin_mapping: false,
    admin_username: "",
    message: "解析中",
    loading: true,
  }));
  sshOnlineRows.value = rows;

  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  await Promise.all(
    rows.map(async (row) => {
      try {
        const r = await client.adminNodeSSHUserPlatform(nodeId, row.local_username);
        row.platform_exists = !!r.platform_exists;
        row.platform_username = String(r.platform_username || "");
        row.real_name = String(r.real_name || "");
        row.mapping_exists = !!r.mapping_exists;
        row.admin_mapping = !!r.admin_mapping;
        row.admin_username = String(r.admin_username || "");
        row.message = String(r.message || "");
      } catch (e: any) {
        row.message = e?.message ?? "解析失败";
      } finally {
        row.loading = false;
      }
    }),
  );
  if (seq !== sshResolveSeq) {
    return;
  }
  sshOnlineRows.value = [...rows];
}

async function openPlatformProfile(username: string) {
  const platformUsername = String(username || "").trim();
  if (!platformUsername) return;
  platformProfileVisible.value = true;
  platformProfileLoading.value = true;
  platformProfileError.value = "";
  platformProfile.value = null;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminPlatformUserDetail(platformUsername);
    platformProfile.value = r.user;
  } catch (e: any) {
    platformProfileError.value = e?.message ?? String(e);
  } finally {
    platformProfileLoading.value = false;
  }
}

function formatSecurityDetails(raw: string): string {
  const text = String(raw || "").trim();
  if (!text) return "-";
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function escapeHTML(raw: string): string {
  return String(raw || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function normalizeSecurityReasonForSummary(reason: string): string {
  return String(reason || "").replace(/\d+(\.\d+)?/g, "#").slice(0, 120);
}

function isSummaryMatchedEvent(event: NodeSecurityEvent, summary: NodeSecurityEventSummary): boolean {
  return String(event.event_type || "").trim() === String(summary.event_type || "").trim() &&
    String(event.severity || "").trim() === String(summary.severity || "").trim() &&
    normalizeSecurityReasonForSummary(event.reason || "") === String(summary.normalized_reason || "");
}

async function showSecurityEventDetail(row: NodeSecurityEvent) {
  await ElMessageBox.alert(
    `<pre style="white-space: pre-wrap;word-break: break-all;margin:0">${formatSecurityDetails(row.details)}</pre>`,
    `事件详情：${row.event_type}`,
    { dangerouslyUseHTMLString: true, confirmButtonText: "关闭" },
  );
}

async function showSecuritySummaryDetail(row: NodeSecurityEventSummary) {
  const matched = (securityEventsRows.value || [])
    .filter((x) => isSummaryMatchedEvent(x, row))
    .sort((a, b) => dayjs(a.created_at).valueOf() - dayjs(b.created_at).valueOf());
  const lines = matched.map((x, idx) => {
    const users = (x.related_usernames || []).join(", ") || "-";
    return `${idx + 1}. ${formatTime(x.created_at)} | 账号: ${users} | 原因: ${x.reason}`;
  });
  const text = [
    `事件类型：${String(row.event_type || "-")}`,
    `事件等级：${String(row.severity || "-")}`,
    `规约原因：${String(row.normalized_reason || "-")}`,
    `归并数量：${Number(row.event_count || 0)} 次`,
    "",
    "出现时间与原始原因：",
    ...(lines.length > 0 ? lines : ["未匹配到原始日志，请先点击“查询”刷新后再查看"]),
  ].join("\n");
  await ElMessageBox.alert(
    `<pre style="white-space: pre-wrap;word-break: break-all;margin:0">${escapeHTML(text)}</pre>`,
    "归并详情",
    { dangerouslyUseHTMLString: true, confirmButtonText: "关闭" },
  );
}

async function showSuspiciousDetail(row: NodeSuspiciousUser) {
  const events = (securityEventsRows.value || [])
    .filter((x) => (x.related_usernames || []).includes(row.username))
    .slice(0, 10);
  const lines = events.map((x) => `${formatTime(x.created_at)} | ${x.event_type} | ${x.reason}`);
  const head = [
    `疑似现象：${String(row.phenomena || "-")}`,
    `疑似挖矿：${row.mining_suspected ? "是" : "否"}`,
    `自动理由：${String(row.reason_hints || "-")}`,
    "",
    "最近事件：",
  ];
  const text = head.concat(lines.length ? lines : ["未找到该账号的详细事件"]).join("\n");
  await ElMessageBox.alert(`<pre style="white-space: pre-wrap;margin:0">${text}</pre>`, `可疑账号：${row.username}`, {
    dangerouslyUseHTMLString: true,
    confirmButtonText: "关闭",
  });
}

async function loadNodeSecurityEvents(nodeId: string, withLoading = true) {
  const id = String(nodeId || "").trim();
  if (!id) return;
  if (withLoading) securityEventsLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const { from, to } = currentSecurityRangeParams();
    const r = await client.adminNodeSecurityEvents(id, {
      eventType: securityEventTypeFilter.value,
      limit: 600,
      summaryLimit: 300,
      from,
      to,
    });
    securityEventsRows.value = Array.isArray(r.events) ? r.events : [];
    securityEventSummariesRows.value = Array.isArray(r.event_summaries) ? r.event_summaries : [];
    securitySummaryNormalizer.value = String(r.summary_normalizer || "event_type + severity + reason(数字归一化)");
    if (detailData.value && String(detailData.value.node.node_id || "").trim() === id) {
      detailData.value.security_events = securityEventsRows.value;
      if (Array.isArray(r.suspicious_users)) {
        detailData.value.suspicious_users = r.suspicious_users;
      }
    }
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    if (withLoading) securityEventsLoading.value = false;
  }
}

async function queryNodeSecurityEvents() {
  if (!detailNodeId.value) return;
  await loadNodeSecurityEvents(detailNodeId.value, true);
}

async function resetNodeSecurityFilters() {
  securityRange.value = buildDefaultSecurityRange();
  securityEventTypeFilter.value = "";
  if (!detailNodeId.value) return;
  await loadNodeSecurityEvents(detailNodeId.value, true);
}

async function blacklistSuspiciousUser(row: NodeSuspiciousUser) {
  const nodeId = String(detailNodeId.value || "").trim();
  if (!nodeId) return;
  let reason = row.reason_hints || row.phenomena || "";
  if (row.mining_suspected) {
    reason = `[疑似挖矿] ${reason}`.trim();
  }
  try {
    const input: any = await ElMessageBox.prompt(`将节点账号 ${row.username} 加入 SSH 黑名单，请填写理由（可留空）`, "拉黑确认", {
      inputValue: reason,
      confirmButtonText: "确认拉黑",
      cancelButtonText: "取消",
    });
    reason = String(input.value || "").trim();
  } catch {
    return;
  }
  blacklistingUserKey.value = `${nodeId}::${row.username}`;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminUpsertBlacklist(nodeId, [row.username], [], reason);
    ElMessage.success(`已将 ${row.username} 拉入节点 ${nodeId} 的 SSH 黑名单`);
    await loadNodeDetail(nodeId, false);
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
  } finally {
    blacklistingUserKey.value = "";
  }
}

async function loadNodeDetail(nodeID: string, withLoading = true) {
  detailNodeId.value = nodeID;
  if (withLoading) detailLoading.value = true;
  detailError.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    detailData.value = await client.adminNodeDetail(nodeID, { minutes: 180, limit: 360 });
    applyNodeStatusSnapshot(detailData.value?.node);
    securityEventsRows.value = detailData.value.security_events || [];
    securityEventSummariesRows.value = [];
    detailLastRefreshAt.value = Date.now();
    await refreshSSHUserMappings(nodeID, detailData.value.latest.ssh_users || []);
    await loadNodeSecurityEvents(nodeID, false);
  } catch (e: any) {
    detailError.value = e?.message ?? String(e);
  } finally {
    if (withLoading) detailLoading.value = false;
  }
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function stopDetailAutoRefresh() {
  if (detailTimer) {
    clearTimeout(detailTimer);
    detailTimer = null;
  }
}

function stopListAutoRefresh() {
  if (listTimer) {
    clearTimeout(listTimer);
    listTimer = null;
  }
}

function startDetailAutoRefresh() {
  stopDetailAutoRefresh();
  detailTimer = setTimeout(async () => {
    if (detailVisible.value && detailNodeId.value) {
      await loadNodeDetail(detailNodeId.value, false);
      startDetailAutoRefresh();
    }
  }, DETAIL_AUTO_REFRESH_MS);
}

function startListAutoRefresh() {
  stopListAutoRefresh();
  listTimer = setTimeout(async () => {
    try {
      if (!loading.value) {
        await reload();
      }
    } finally {
      startListAutoRefresh();
    }
  }, LIST_AUTO_REFRESH_MS);
}

async function openNodeDetail(row: NodeStatus) {
  securityRange.value = buildDefaultSecurityRange();
  securityEventTypeFilter.value = "";
  securityShowSummary.value = true;
  detailVisible.value = true;
  await loadNodeDetail(row.node_id, true);
  startDetailAutoRefresh();
}

async function openNodeDetailById(nodeID: string) {
  const id = String(nodeID || "").trim();
  if (!id) return;
  securityRange.value = buildDefaultSecurityRange();
  securityEventTypeFilter.value = "";
  securityShowSummary.value = true;
  const row = rows.value.find((x) => String(x.node_id || "").trim() === id);
  if (row) {
    await openNodeDetail(row);
    return;
  }
  detailVisible.value = true;
  await loadNodeDetail(id, true);
  startDetailAutoRefresh();
}

async function disconnectAllSSH(row: NodeStatus) {
  const count = row.ssh_active_count || 0;
  const nodeId = row.node_id;
  try {
    await ElMessageBox.confirm(
      `确认清除节点 ${nodeId} 的 SSH 状态吗？\n当前检测到 SSH 登录用户数：${count}\n执行后将强制断开会话，用户需重新连接。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  disconnectingNodeId.value = nodeId;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminDisconnectNodeSSH(nodeId);
    ElMessage.success(r.message || "已下发清理指令");
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    disconnectingNodeId.value = "";
  }
}

async function killAllUserProcessesByNode(nodeId: string, localUserCount?: number) {
  nodeId = String(nodeId || "").trim();
  if (!nodeId) return;
  const countText = typeof localUserCount === "number" && localUserCount >= 0 ? `\n节点本地用户数：${localUserCount}` : "";
  try {
    await ElMessageBox.confirm(
      `确认强制清除节点 ${nodeId} 的全部用户进程吗？\n覆盖范围：节点内全部本地用户（含未上线用户）。${countText}\n该操作会立即发送 KILL 指令，请仅在紧急情况下执行。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  killingProcNodeId.value = nodeId;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminKillAllUserProcesses(nodeId);
    ElMessage.success(r.message || "已下发清理指令");
    await reload();
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    killingProcNodeId.value = "";
  }
}

async function killAllUserProcesses(row: NodeStatus) {
  const nodeId = String(row.node_id || "").trim();
  const localUserCount = detailVisible.value && detailNodeId.value === nodeId ? (detailData.value?.local_users || []).length : undefined;
  await killAllUserProcessesByNode(nodeId, localUserCount);
}

async function killAllDetailUsersProcesses() {
  const nodeId = String(detailNodeId.value || "").trim();
  if (!nodeId) return;
  if (detailHasProtectedAdminMappings.value) {
    ElMessage.warning("存在管理员映射账号，高级用户不能执行全体清进程");
    return;
  }
  const localUserCount = (detailData.value?.local_users || []).length;
  await killAllUserProcessesByNode(nodeId, localUserCount);
}

async function saveDetailUserLimitsFromDialog() {
  const nodeId = String(userLimitNodeId.value || "").trim();
  const local = String(userLimitLocalUsername.value || "").trim();
  if (!nodeId || !local) return;
  const cpuEnabled = !!userLimitCPUEnabled.value;
  const memoryEnabled = !!userLimitMemoryEnabled.value;
  const gpuEnabled = !!userLimitGPUEnabled.value;
  const cpuPercent = Number(userLimitCPUPercent.value || 0);
  const memoryGB = Number(userLimitMemoryGB.value || 0);
  const gpuIndices = normalizeGPUIndexList((userLimitVisibleGPUIndices.value || []).map((x) => Number(x)));
  if (cpuEnabled && (!Number.isFinite(cpuPercent) || cpuPercent <= 0 || cpuPercent > 100)) {
    ElMessage.error("CPU 限制比例必须在 1~100 之间");
    return;
  }
  if (memoryEnabled && (!Number.isFinite(memoryGB) || memoryGB <= 0 || memoryGB > 4096)) {
    ElMessage.error("内存限制必须在 0~4096 GB 之间");
    return;
  }
  if (gpuEnabled && gpuIndices.length === 0) {
    ElMessage.error("请至少选择一个可见 GPU");
    return;
  }
  userLimitSaving.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const reason = String(userLimitReason.value || "").trim();
    if (cpuEnabled) {
      await client.adminSetNodeUserCPULimit(nodeId, {
        local_username: local,
        cpu_quota_percent: Number(cpuPercent.toFixed(1)),
        reason,
      });
    } else {
      await deleteCPUUserLimitSafe(nodeId, local);
    }
    if (memoryEnabled) {
      await client.adminSetNodeUserMemoryLimit(nodeId, {
        local_username: local,
        memory_limit_gb: Number(memoryGB.toFixed(1)),
        reason,
      });
    } else {
      await deleteMemoryUserLimitSafe(nodeId, local);
    }
    if (gpuEnabled) {
      await client.adminSetNodeUserGPUVisibility(nodeId, {
        local_username: local,
        gpu_indices: gpuIndices,
        reason,
      });
    } else {
      await deleteGPUVisibilitySafe(nodeId, local);
    }
    userLimitDialogVisible.value = false;
    ElMessage.success(`已更新 ${local} 的限制设置`);
    await wait(1200);
    await loadNodeDetail(nodeId, false);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    userLimitSaving.value = false;
  }
}

async function clearDetailUserLimits(localUsername: string) {
  const nodeId = String(detailNodeId.value || "").trim();
  const local = String(localUsername || "").trim();
  if (!nodeId || !local) return;
  try {
    await ElMessageBox.confirm(
      `确认解除节点 ${nodeId} 用户 ${local} 的手动限速吗？`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认解除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  userLimitRemovingKey.value = `${nodeId}::${local}`;
  error.value = "";
  try {
    await deleteCPUUserLimitSafe(nodeId, local);
    await deleteMemoryUserLimitSafe(nodeId, local);
    await deleteGPUVisibilitySafe(nodeId, local);
    ElMessage.success(`已解除 ${local} 的手动限制`);
    await wait(1200);
    await loadNodeDetail(nodeId, false);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    userLimitRemovingKey.value = "";
  }
}

async function killDetailUserProcesses(localUsername: string) {
  const nodeId = String(detailNodeId.value || "").trim();
  const user = String(localUsername || "").trim();
  if (!nodeId || !user) return;
  if (authState.role === "power_user") {
    const localRows = detailData.value?.local_users || [];
    const target = localRows.find((x) => String(x.local_username || "").trim() === user);
    if (target?.admin_mapping) {
      ElMessage.warning("高级用户不能操作管理员映射账号");
      return;
    }
  }
  try {
    await ElMessageBox.confirm(
      `确认强制清理节点 ${nodeId} 的用户 ${user} 全部进程吗？\n该操作会立即发送 KILL 指令，请仅在必要时使用。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  killingUserProcKey.value = `${nodeId}::${user}`;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminKillNodeUserProcesses(nodeId, user);
    ElMessage.success(r.message || `已下发用户 ${user} 的清理进程指令`);
    await wait(1200);
    await loadNodeDetail(nodeId, false);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    killingUserProcKey.value = "";
  }
}

async function syncNodeNow(nodeId: string) {
  if (!nodeId) return;
  syncingNodeId.value = nodeId;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminSyncNodeNow(nodeId);
    ElMessage.success(r.message || "已下发立即同步指令");

    // action poll 默认 1s，这里等待后刷新，优先拿到新快照和 SSH 用户列表。
    await wait(1200);
    await reload();
    if (detailVisible.value && detailNodeId.value === nodeId) {
      await loadNodeDetail(nodeId, false);
      startDetailAutoRefresh();
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    syncingNodeId.value = "";
  }
}

async function syncCurrentNode() {
  if (!detailNodeId.value) {
    ElMessage.warning("请先点击节点 ID 进入详情，再执行立即同步");
    return;
  }
  await syncNodeNow(detailNodeId.value);
}

async function refreshDetailNow() {
  if (!detailNodeId.value) return;
  await loadNodeDetail(detailNodeId.value, false);
  startDetailAutoRefresh();
}

async function disconnectSSHUser(localUsername: string) {
  const nodeId = String(detailNodeId.value || "").trim();
  const user = String(localUsername || "").trim();
  if (!nodeId || !user) return;
  if (authState.role === "power_user") {
    const online = sshOnlineRows.value.find((x) => String(x.local_username || "").trim() === user);
    if (online?.admin_mapping) {
      ElMessage.warning("高级用户不能操作管理员映射账号");
      return;
    }
  }
  try {
    await ElMessageBox.confirm(
      `确认强制下线节点 ${nodeId} 的用户 ${user} 吗？\n执行后该用户当前 SSH 会话会被断开，需要重新登录。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认下线", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }

  disconnectingUserKey.value = `${nodeId}::${user}`;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminDisconnectNodeSSHUser(nodeId, user);
    ElMessage.success(r.message || `已下发 ${user} 的强制下线指令`);
    await wait(1200);
    await loadNodeDetail(nodeId, false);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    disconnectingUserKey.value = "";
  }
}

reload();
startListAutoRefresh();
onBeforeUnmount(() => {
  stopDetailAutoRefresh();
  stopListAutoRefresh();
});
</script>

<style scoped>
.page-container {
  width: 100%;
  max-width: none;
  margin: 0 auto;
  padding: 0 8px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
  padding: 24px;
  background: var(--bg-card);
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-color);
  box-shadow: var(--shadow-md);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 12px;
  background: var(--primary-gradient);
  box-shadow: var(--shadow-md);
}

.page-title {
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
  margin: 0;
}

.page-subtitle {
  font-size: 14px;
  color: var(--text-tertiary);
  margin: 4px 0 0 0;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.icon-action-btn {
  color: var(--primary-color) !important;
  padding: 4px !important;
  min-height: 28px;
}

.icon-action-btn:hover {
  color: var(--primary-color-hover) !important;
  background: transparent !important;
}

.refresh-time-text {
  margin-right: 4px;
}

.error-alert {
  margin-bottom: 24px;
}

.risk-banner {
  margin-bottom: 18px;
}

.risk-banner-list {
  margin-top: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.risk-chip {
  border: 1px solid #f59e0b;
  background: #fffbeb;
  color: #92400e;
  border-radius: 999px;
  padding: 4px 10px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  font-size: 12px;
  line-height: 1;
}

.risk-chip:hover {
  background: #fef3c7;
}

.risk-chip-emoji {
  font-size: 14px;
}

.risk-chip-meta {
  color: #b45309;
}

.security-filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  margin: 8px 0 12px;
}

.security-normalizer-alert {
  margin-bottom: 10px;
}

.points-policy-help {
  width: 100%;
}

.disk-quota-alert {
  width: 100%;
}

.disk-quota-mounts {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.points-policy-help-lines {
  display: grid;
  gap: 4px;
  margin-top: 4px;
  font-size: 13px;
  line-height: 1.5;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 20px;
  margin-bottom: 24px;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-md);
  transition: all 0.3s ease;
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: var(--shadow-lg);
  border-color: var(--primary-color);
}

.stat-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 12px;
  color: white;
  box-shadow: var(--shadow-md);
}

.stat-content {
  flex: 1;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1;
}

.stat-label {
  font-size: 13px;
  color: var(--text-tertiary);
  margin-top: 6px;
}

.table-card {
  animation: fadeIn 0.5s ease;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
}

.node-id-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  font-weight: 500;
}

.node-id-head {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.risk-emoji {
  font-size: 16px;
  line-height: 1;
}

.node-status-cluster {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
}

.node-status-tag {
  margin-left: 0;
}

.node-status-tag-clickable {
  cursor: pointer;
}

.node-status-tag-clickable:hover {
  border-color: var(--primary-color);
  color: var(--primary-color);
}

.node-guard-switch-wrap {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.node-switch-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.node-switch-actions {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.node-switch-label {
  color: var(--text-secondary);
}

.node-visibility-btn {
  padding: 0 !important;
}

.exclusive-gpu-assign-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.exclusive-gpu-user-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 8px 10px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fafafa;
}

.exclusive-gpu-user {
  min-width: 120px;
  font-weight: 600;
  color: #1f2937;
}

.sync-status-btn {
  color: #fff !important;
}

.sync-status-btn:hover,
.sync-status-btn:focus {
  color: #fff !important;
}

.sync-status-btn.is-disabled {
  color: #fff !important;
}

.node-id-link {
  padding: 0 !important;
  margin: 0 !important;
  border: none !important;
  background: transparent !important;
  color: var(--primary-color) !important;
  font-weight: 600;
}

.node-id-link:hover {
  background: transparent !important;
  text-decoration: underline;
}

.time-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--text-secondary);
}

.cost-cell {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  font-weight: 600;
  color: var(--warning-color);
}

.agent-version-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.service-tooltip {
  display: grid;
  gap: 4px;
  max-width: 320px;
  font-size: 12px;
  line-height: 1.45;
}

.node-service-detail {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.node-service-detail-time {
  color: var(--text-tertiary);
  font-size: 12px;
}

.agent-version-outdated-icon {
  color: #f59e0b;
  font-size: 14px;
}

.ssh-users-wrap {
  margin-top: 14px;
}

.node-price-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.detail-actions {
  margin-bottom: 10px;
  display: flex;
  gap: 8px;
}

.ssh-users-title {
  margin-bottom: 8px;
  font-weight: 700;
  color: var(--text-primary);
}
.mapping-state-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.mapping-state-tip {
  font-size: 12px;
  line-height: 1.4;
  color: var(--text-tertiary);
}
.section-inline-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.section-inline-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.section-inline-title :deep(svg) {
  width: 16px;
  height: 16px;
}

.ssh-users-empty {
  color: var(--text-tertiary);
}

.admin-map-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  margin-left: 6px;
  border-radius: 999px;
  background: #dc2626;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
  cursor: default;
  user-select: none;
}

.home-used-cell {
  display: block;
  text-align: left;
}

.home-used-danger {
  color: #ef4444 !important;
  font-weight: 700;
}

.cpu-limit-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.cpu-limit-tag-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.cpu-limit-reason {
  color: var(--text-tertiary);
  font-size: 12px;
  line-height: 1.25;
  word-break: break-all;
}

.cpu-limit-editor {
  display: flex;
  align-items: center;
  gap: 6px;
}

.charts-grid {
  margin-top: 14px;
  display: grid;
  grid-template-columns: repeat(2, minmax(280px, 1fr));
  gap: 12px;
}

.chart-card :deep(.el-card__body) {
  padding-top: 8px;
}
.chart-head {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.chart-value {
  font-size: 20px;
  font-weight: 700;
  margin-bottom: 6px;
  color: var(--text-primary);
}

.chart-svg {
  width: 100%;
  height: 130px;
  border-radius: 8px;
  background: linear-gradient(180deg, rgba(15, 118, 110, 0.06) 0%, rgba(15, 118, 110, 0.01) 100%);
}

@media (max-width: 1200px) {
  .charts-grid {
    grid-template-columns: 1fr;
  }
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
