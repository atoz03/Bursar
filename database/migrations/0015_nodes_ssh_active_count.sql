-- 0015_nodes_ssh_active_count.sql
-- 节点当前 SSH 登录人数（由 node-agent 上报 who 去重结果）

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS ssh_active_count INT NOT NULL DEFAULT 0;
