CREATE TABLE IF NOT EXISTS `mosaicplane_oidc_user` (
  `id` varchar(36) NOT NULL COMMENT 'primary key',
  `issuer` varchar(512) NOT NULL COMMENT 'OIDC issuer URL',
  `subject` varchar(255) NOT NULL COMMENT 'OIDC subject identifier',
  `username` varchar(255) NOT NULL DEFAULT '' COMMENT 'stable username',
  `nickname` varchar(255) NOT NULL DEFAULT '' COMMENT 'display name',
  `email` varchar(320) NOT NULL DEFAULT '' COMMENT 'email address',
  `phone` varchar(64) NOT NULL DEFAULT '' COMMENT 'phone number',
  `avatar` varchar(1024) NOT NULL DEFAULT '' COMMENT 'avatar URL',
  `last_login_at` datetime(3) NOT NULL COMMENT 'last successful login time',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'created time',
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'updated time',
  `soft_deleted_at` bigint(20) NOT NULL DEFAULT 0 COMMENT 'soft deletion timestamp',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_mosaicplane_oidc_identity` (`issuer`(255), `subject`),
  KEY `idx_mosaicplane_oidc_user_username` (`username`),
  KEY `idx_mosaicplane_oidc_user_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='MosaicPlane OIDC user directory';
