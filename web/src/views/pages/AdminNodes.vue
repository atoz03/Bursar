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
              <el-button link class="node-id-link" @click="openNodeDetail(row)">{{ row.node_id }}</el-button>
              <span
                v-if="hasNodeSecurityRisk(row)"
                class="risk-emoji"
                :title="nodeRiskTooltip(row)"
              >
                {{ nodeRiskEmoji(row) }}
              </span>
              <el-tag
                size="small"
                effect="plain"
                :type="nodeStatusTagType(row)"
                class="node-status-tag"
              >
                {{ nodeStatusText(row) }}
              </el-tag>
              <el-tag v-if="row.ssh_exclusive_enabled" size="small" type="danger" effect="dark" class="node-status-tag">
                独享中
              </el-tag>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="策略" width="250">
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
              <div class="node-switch-actions">
                <el-button
                  v-if="canManageNodes"
                  size="small"
                  link
                  type="primary"
                  class="node-visibility-btn"
                  @click="openNodePriceDialog(row)"
                >
                  单卡积分
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
          <el-descriptions-item label="CPU型号">{{ detailData.node.cpu_model || "-" }}</el-descriptions-item>
          <el-descriptions-item label="CPU数量">{{ detailData.node.cpu_count || 0 }}</el-descriptions-item>
          <el-descriptions-item label="CPU利用率(估算)">{{ cpuUtilNow.toFixed(2) }}%</el-descriptions-item>
          <el-descriptions-item label="GPU型号">{{ detailData.node.gpu_model || "-" }}</el-descriptions-item>
          <el-descriptions-item label="GPU数量">{{ detailData.node.gpu_count || 0 }}</el-descriptions-item>
          <el-descriptions-item label="GPU活跃度(估算)">{{ gpuUtilNow.toFixed(2) }}%</el-descriptions-item>
          <el-descriptions-item label="系统版本">{{ detailData.node.os_version || "-" }}</el-descriptions-item>
          <el-descriptions-item label="内核版本">{{ detailData.node.kernel_version || "-" }}</el-descriptions-item>
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
          <el-descriptions-item label="节点积分单价">
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
                  :disabled="row.loading || !canManageNodes"
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
          <div class="ssh-users-title section-inline-title">
            <el-icon><UserFilled /></el-icon>
            <span>节点内全部本地用户（含映射状态）</span>
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
          <el-table
            :data="detailData.security_events || []"
            size="small"
            stripe
            style="width: 100%"
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
          <el-table-column prop="updated_at" label="更新时间" min-width="180" />
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

    <el-dialog v-model="nodeExclusiveVisible" title="节点 SSH 独享设置" width="620px">
      <el-form label-width="110px">
        <el-form-item label="节点编号">
          <el-text>{{ nodeExclusiveNodeId || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="独享开关">
          <el-switch v-model="nodeExclusiveEnabled" inline-prompt active-text="开启" inactive-text="关闭" />
          <el-text type="info" size="small" style="margin-left: 8px">
            开启后，仅“独享用户 + 白名单 + 豁免”可登录 SSH；黑名单仍最高优先级
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
        <el-form-item label="规则说明">
          <el-alert
            type="warning"
            show-icon
            :closable="false"
            title="判定顺序：豁免放行 → 黑名单拒绝 →（若独享开启）仅独享用户放行；白名单可绕过独享；其余拒绝。"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="nodeExclusiveVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodeExclusiveSaving" @click="saveNodeExclusive">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="nodePriceVisible" title="节点单卡积分设置" width="700px">
      <el-form label-width="130px">
        <el-form-item label="节点ID">
          <el-text>{{ nodePriceNodeId || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="节点单价">
          <el-input-number
            v-model="nodePricePerMinute"
            :min="0"
            :step="0.01"
            :precision="4"
            style="width: 220px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">单位：积分 / GPU·分钟</el-text>
        </el-form-item>
        <el-form-item label="说明">
          <el-alert
            type="info"
            show-icon
            :closable="false"
            title="节点单价仅对当前节点生效，不再按卡型或全局价格模式切换。"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="nodePriceVisible = false">取消</el-button>
        <el-button type="primary" :loading="nodePriceSaving" @click="saveNodePrice">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiClient, type NodeDetailResp, type NodeRuntimeSnapshot, type NodeSecurityEvent, type NodeStatus, type NodeSuspiciousUser } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { Monitor, Refresh, Cpu, Coin, Clock, List, Document, User, UserFilled, WarningFilled } from "@element-plus/icons-vue";
import dayjs from "dayjs";

const loading = ref(false);
const error = ref("");
const rows = ref<NodeStatus[]>([]);
const disconnectingNodeId = ref("");
const killingProcNodeId = ref("");
const syncingNodeId = ref("");
const guardUpdatingNodeId = ref("");
const pointsUpdatingNodeId = ref("");
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
const nodeExclusiveUsers = ref<string[]>([]);
const nodeExclusiveCandidates = ref<string[]>([]);
const nodePriceVisible = ref(false);
const nodePriceSaving = ref(false);
const nodePriceNodeId = ref("");
const nodePricePerMinute = ref(0.1);
const detailVisible = ref(false);
const detailLoading = ref(false);
const detailError = ref("");
const detailNodeId = ref("");
const detailData = ref<NodeDetailResp | null>(null);
const lastRefreshAt = ref("");
const detailLastRefreshAt = ref("");
const disconnectingUserKey = ref("");
const blacklistingUserKey = ref("");
const sshOnlineRows = ref<Array<{
  local_username: string;
  platform_exists: boolean;
  platform_username: string;
  real_name: string;
  mapping_exists: boolean;
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
  node_accounts: Array<{ node_id: string; local_username: string; updated_at: string }>;
} | null>(null);
let detailTimer: ReturnType<typeof setTimeout> | null = null;
let sshResolveSeq = 0;
const DETAIL_AUTO_REFRESH_MS = 5 * 60 * 1000;

const totalGpuProcesses = computed(() => rows.value.reduce((sum, node) => sum + node.gpu_process_count, 0));
const totalCpuProcesses = computed(() => rows.value.reduce((sum, node) => sum + node.cpu_process_count, 0));
const totalCost = computed(() => rows.value.reduce((sum, node) => sum + node.cost_total, 0));
const onlineNodeCount = computed(() => rows.value.filter((node) => getNodeStatus(node) === "online").length);
const sortedRows = computed(() => [...rows.value].sort((a, b) => nodeIdSortValue(b.node_id) - nodeIdSortValue(a.node_id)));
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

function formatTime(time: string): string {
  if (!time) return "-";
  return dayjs(time).format("YYYY-MM-DD HH:mm:ss");
}

function fmtGB(v?: number): string {
  const n = Number(v || 0);
  if (!Number.isFinite(n) || n <= 0) return "-";
  return `${n.toFixed(2)} GB`;
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

function getNodeStatus(node: NodeStatus): "online" | "offline" {
  const candidates = [
    dayjs(String(node?.last_seen_at || "")),
    dayjs(String(node?.last_report_ts || "")),
    dayjs(String(node?.updated_at || "")),
  ].filter((t) => t.isValid());
  // 按你的使用习惯：只要节点有可用上报时间，就视为在线。
  // 避免“能看到节点信息却被判离线”的体验问题。
  if (candidates.length > 0) return "online";
  if (Number(node?.gpu_process_count || 0) > 0 || Number(node?.cpu_process_count || 0) > 0 || Number(node?.ssh_active_count || 0) > 0) {
    return "online";
  }
  return "offline";
}

function nodeStatusText(node: NodeStatus): string {
  const st = getNodeStatus(node);
  if (st === "online") return "上报在线";
  return "上报离线";
}

function nodeStatusTagType(node: NodeStatus): "success" | "info" | "warning" {
  const st = getNodeStatus(node);
  if (st === "online") return "success";
  return "info";
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
    lastRefreshAt.value = new Date().toISOString();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
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
    row.points_intercept_enabled = !!r.enabled;
    ElMessage.success(r.enabled ? `节点 ${nodeId} 已开启正常扣分与限速` : `节点 ${nodeId} 已关闭扣分与限速（仅记录使用）`);
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
  nodeExclusiveUsers.value = [];
  nodeExclusiveCandidates.value = [];
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodeSSHExclusive(nodeId);
    nodeExclusiveEnabled.value = !!r.enabled;
    nodeExclusiveUsers.value = Array.isArray(r.exclusive_users) ? [...r.exclusive_users] : [];
    nodeExclusiveCandidates.value = Array.isArray(r.candidate_local_users) ? [...r.candidate_local_users] : [];
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
    nodeExclusiveVisible.value = false;
  }
}

async function saveNodeExclusive() {
  const nodeId = String(nodeExclusiveNodeId.value || "").trim();
  if (!nodeId) return;
  nodeExclusiveSaving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminSetNodeSSHExclusive(nodeId, {
      enabled: !!nodeExclusiveEnabled.value,
      exclusive_users: nodeExclusiveEnabled.value ? nodeExclusiveUsers.value : [],
    });
    ElMessage.success("节点独享策略保存成功，SSH 策略将立即同步");
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

async function openNodePriceDialog(row: NodeStatus) {
  const nodeId = String(row.node_id || "").trim();
  if (!nodeId) return;
  nodePriceNodeId.value = nodeId;
  nodePriceVisible.value = true;
  nodePriceSaving.value = false;
  nodePricePerMinute.value = Number(row.node_price_per_minute ?? 0.1);
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminNodePrice(nodeId);
    nodePricePerMinute.value = Number(r.price_per_minute ?? r.default_price_per_minute ?? 0.1);
  } catch (e: any) {
    ElMessage.error(e?.message ?? String(e));
    nodePriceVisible.value = false;
  }
}

async function saveNodePrice() {
  const nodeId = String(nodePriceNodeId.value || "").trim();
  if (!nodeId) return;
  if (!Number.isFinite(nodePricePerMinute.value) || Number(nodePricePerMinute.value) < 0) {
    ElMessage.error("请填写合法的节点单卡积分（>= 0）");
    return;
  }
  nodePriceSaving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminSetNodePrice(nodeId, { price_per_minute: Number(nodePricePerMinute.value) });
    ElMessage.success("节点单卡积分已更新");
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

async function showSecurityEventDetail(row: NodeSecurityEvent) {
  await ElMessageBox.alert(
    `<pre style="white-space: pre-wrap;word-break: break-all;margin:0">${formatSecurityDetails(row.details)}</pre>`,
    `事件详情：${row.event_type}`,
    { dangerouslyUseHTMLString: true, confirmButtonText: "关闭" },
  );
}

async function showSuspiciousDetail(row: NodeSuspiciousUser) {
  const events = (detailData.value?.security_events || [])
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
    detailLastRefreshAt.value = new Date().toISOString();
    await refreshSSHUserMappings(nodeID, detailData.value.latest.ssh_users || []);
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

function startDetailAutoRefresh() {
  stopDetailAutoRefresh();
  detailTimer = setTimeout(async () => {
    if (detailVisible.value && detailNodeId.value) {
      await loadNodeDetail(detailNodeId.value, false);
      startDetailAutoRefresh();
    }
  }, DETAIL_AUTO_REFRESH_MS);
}

async function openNodeDetail(row: NodeStatus) {
  detailVisible.value = true;
  await loadNodeDetail(row.node_id, true);
  startDetailAutoRefresh();
}

async function openNodeDetailById(nodeID: string) {
  const id = String(nodeID || "").trim();
  if (!id) return;
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

async function killAllUserProcesses(row: NodeStatus) {
  const nodeId = row.node_id;
  try {
    await ElMessageBox.confirm(
      `确认清除节点 ${nodeId} 的全部用户进程吗？\n该操作会强制结束节点上所有普通节点用户进程，仅用于紧急清理，避免绕过注册审计。`,
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
onBeforeUnmount(() => {
  stopDetailAutoRefresh();
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
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.risk-emoji {
  font-size: 16px;
  line-height: 1;
}

.node-status-tag {
  margin-left: 2px;
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
.section-inline-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.section-inline-title :deep(svg) {
  width: 16px;
  height: 16px;
}

.ssh-users-empty {
  color: var(--text-tertiary);
}

.home-used-cell {
  display: block;
  text-align: left;
}

.home-used-danger {
  color: #ef4444 !important;
  font-weight: 700;
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
