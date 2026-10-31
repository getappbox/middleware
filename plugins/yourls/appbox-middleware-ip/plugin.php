<?php
/*
Plugin Name: AppBox Middleware Client IP
Description: Accept one uploader IP from configured AppBox middleware peers for API calls.
Version: 1.0.0
*/

if (!defined('YOURLS_ABSPATH')) {
    die();
}

yourls_add_filter('get_IP', 'appbox_middleware_client_ip', 99);

function appbox_middleware_normalize_ip($value) {
    if (!is_string($value) || filter_var($value, FILTER_VALIDATE_IP) === false) {
        return '';
    }
    $packed = inet_pton($value);
    if (strlen($packed) === 16 && substr($packed, 0, 12) === str_repeat("\0", 10) . "\xff\xff") {
        $packed = substr($packed, 12);
    }
    return inet_ntop($packed);
}

function appbox_middleware_trusted_ips() {
    $configured = getenv('APPBOX_MIDDLEWARE_TRUSTED_IPS');
    $trusted = [];
    foreach (explode(',', is_string($configured) ? $configured : '') as $value) {
        $ip = appbox_middleware_normalize_ip(trim($value));
        if ($ip !== '') {
            $trusted[] = $ip;
        }
    }
    return $trusted;
}

function appbox_middleware_client_ip($ip) {
    if (!defined('YOURLS_API') || !YOURLS_API) {
        return $ip;
    }
    $peer = appbox_middleware_normalize_ip($_SERVER['REMOTE_ADDR'] ?? '');
    if (!in_array($peer, appbox_middleware_trusted_ips(), true)) {
        return $peer;
    }
    $forwarded = $_SERVER['HTTP_X_FORWARDED_FOR'] ?? '';
    $client = appbox_middleware_normalize_ip(is_string($forwarded) ? trim($forwarded, " \t") : '');
    return $client !== '' ? $client : $peer;
}
