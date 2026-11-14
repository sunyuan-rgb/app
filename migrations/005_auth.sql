-- 真实认证流（2026-11-13）：登录/注册需校验密码，预置账号补充默认密码。
-- 口令存储采用 SHA2(username:password)（用户名作盐）。
-- 说明：生产环境应使用 bcrypt/argon2id 等慢哈希并走独立密钥管理；
-- 教学环境取标准库 crypto/sha256 以保持零第三方依赖，边界在注释中如实声明。
ALTER TABLE user_account
    ADD COLUMN password_hash CHAR(64) NULL COMMENT 'SHA2(username:password)，登录校验用';

-- 预置账号默认口令：user1~user5=123456，agent1=agent123，ops1=ops123
UPDATE user_account SET password_hash = SHA2(CONCAT(username, ':', '123456'), 256)
 WHERE username IN ('user1','user2','user3','user4','user5');
UPDATE user_account SET password_hash = SHA2(CONCAT(username, ':', 'agent123'), 256)
 WHERE username = 'agent1';
UPDATE user_account SET password_hash = SHA2(CONCAT(username, ':', 'ops123'), 256)
 WHERE username = 'ops1';
