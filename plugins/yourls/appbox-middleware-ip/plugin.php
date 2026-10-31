<?php
/*
Plugin Name: AppBox Middleware Client IP
Plugin URI: https://github.com/getappbox/middleware/tree/main/plugins/yourls
Description: Accept one uploader IP from configured AppBox middleware peers for API calls.
Version: 1.1.0
Author: Vineet Choudhary
Author URI: https://github.com/vineetchoudhary
*/

if (!defined('YOURLS_ABSPATH')) {
    return;
}

if (!function_exists('appbox_middleware_client_ip') &&
    !function_exists('appbox_middleware_normalize_ip') &&
    !function_exists('appbox_middleware_trusted_ips') &&
    !function_exists('appbox_middleware_log_failure')) {

    function appbox_middleware_log_failure() {
        try {
            error_log('AppBox middleware IP plugin: attribution unavailable for this request.');
        } catch (\Throwable $error) {
        }
    }

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
        $peer = '';
        try {
            $peer = appbox_middleware_normalize_ip($_SERVER['REMOTE_ADDR'] ?? '');
            if (!in_array($peer, appbox_middleware_trusted_ips(), true)) {
                return $peer;
            }
            $forwarded = $_SERVER['HTTP_X_FORWARDED_FOR'] ?? '';
            $client = appbox_middleware_normalize_ip(is_string($forwarded) ? trim($forwarded, " \t") : '');
            return $client !== '' ? $client : $peer;
        } catch (\Throwable $error) {
            appbox_middleware_log_failure();
            return $peer;
        }
    }

    try {
        yourls_add_filter('get_IP', 'appbox_middleware_client_ip', 99);
    } catch (\Throwable $error) {
        appbox_middleware_log_failure();
    }
}
