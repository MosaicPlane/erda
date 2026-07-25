CREATE TABLE IF NOT EXISTS `mosaicplane_oidc_user` (
  `id` varchar(36) NOT NULL,
  `issuer` varchar(512) NOT NULL,
  `subject` varchar(255) NOT NULL,
  `username` varchar(255) NOT NULL DEFAULT '',
  `nickname` varchar(255) NOT NULL DEFAULT '',
  `email` varchar(320) NOT NULL DEFAULT '',
  `phone` varchar(64) NOT NULL DEFAULT '',
  `avatar` varchar(1024) NOT NULL DEFAULT '',
  `last_login_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_mosaicplane_oidc_identity` (`issuer`(255), `subject`),
  KEY `idx_mosaicplane_oidc_user_username` (`username`),
  KEY `idx_mosaicplane_oidc_user_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
