-- 记录“注册申请退回”通知邮件的发送结果，供后台退回列表展示。
ALTER TABLE registration_requests
  ADD COLUMN IF NOT EXISTS reject_notify_mail_checked BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE registration_requests
  ADD COLUMN IF NOT EXISTS reject_notify_mail_sent BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE registration_requests
  ADD COLUMN IF NOT EXISTS reject_notify_mail_error TEXT NOT NULL DEFAULT '';
