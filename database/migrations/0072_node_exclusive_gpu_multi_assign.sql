DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'node_exclusive_gpu_users'::regclass
          AND conname = 'node_exclusive_gpu_users_pkey'
    ) THEN
        ALTER TABLE node_exclusive_gpu_users
            DROP CONSTRAINT node_exclusive_gpu_users_pkey;
    END IF;

    ALTER TABLE node_exclusive_gpu_users
        ADD CONSTRAINT node_exclusive_gpu_users_pkey
        PRIMARY KEY (node_id, local_username, gpu_index);
END $$;
