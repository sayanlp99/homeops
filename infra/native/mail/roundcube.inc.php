<?php
// HomeOps overrides for Roundcube (included from /etc/roundcube/config.inc.php).
// Stalwart runs on this machine; its certificate is self-signed, so loopback
// connections skip verification. Nothing here leaves 127.0.0.1.
$loopback = ['ssl' => ['verify_peer' => false, 'verify_peer_name' => false, 'allow_self_signed' => true]];
$config['imap_host'] = 'ssl://127.0.0.1:993';
$config['imap_conn_options'] = $loopback;
$config['smtp_host'] = 'tls://127.0.0.1:587';
$config['smtp_conn_options'] = $loopback;
$config['smtp_user'] = '%u';
$config['smtp_pass'] = '%p';
$config['username_domain'] = '__DOMAIN__';   // sign in with just "me"
$config['mail_domain'] = '__DOMAIN__';
$config['product_name'] = '__DOMAIN__ Mail';
$config['support_url'] = '';
$config['skin'] = 'elastic';
$config['des_key'] = trim(file_get_contents('/etc/roundcube-homeops/des.key'));
$config['db_dsnw'] = 'sqlite:////var/lib/roundcube-homeops/roundcube.db?mode=0640';
$config['temp_dir'] = '/var/lib/roundcube-homeops/temp/';
$config['plugins'] = ['archive', 'zipdownload', 'managesieve', 'markasjunk'];
// Served through Cloudflare Tunnel: trust its forwarded client IP, cookies are HTTPS-only.
$config['proxy_whitelist'] = ['127.0.0.1', '::1'];
$config['use_https'] = true;
$config['session_samesite'] = 'Strict';
$config['session_lifetime'] = 60 * 24 * 7;
$config['login_rate_limit'] = 5;              // failed logins per minute per client IP
$config['ip_check'] = false;                  // Cloudflare may change the edge IP mid-session
$config['enable_installer'] = false;
$config['x_frame_options'] = 'sameorigin';
$config['identities_level'] = 3;              // can't send as an arbitrary From address
$config['managesieve_host'] = '127.0.0.1';
$config['managesieve_port'] = 4190;
$config['log_driver'] = 'syslog';
