-- 可配置机柜功率看板：保存机柜容量、槽位布局与节点核算功率。
-- 不预置任何机柜或节点，管理员在“机柜功率”页面自行维护。
CREATE TABLE IF NOT EXISTS power_racks (
    rack_code VARCHAR(32) PRIMARY KEY,
    name VARCHAR(80) NOT NULL,
    capacity_w INT NOT NULL CHECK (capacity_w > 0),
    slot_count INT NOT NULL DEFAULT 5 CHECK (slot_count > 0 AND slot_count <= 60),
    location VARCHAR(160) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS power_rack_nodes (
    node_id VARCHAR(50) PRIMARY KEY,
    rack_code VARCHAR(32) NOT NULL REFERENCES power_racks(rack_code) ON DELETE CASCADE,
    slot_number INT NOT NULL CHECK (slot_number > 0 AND slot_number <= 60),
    allocated_power_w INT NOT NULL CHECK (allocated_power_w >= 0),
    device_label VARCHAR(160) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(50) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(rack_code, slot_number)
);

CREATE INDEX IF NOT EXISTS idx_power_rack_nodes_rack_slot
ON power_rack_nodes(rack_code, slot_number);
