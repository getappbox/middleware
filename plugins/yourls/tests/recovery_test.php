<?php

if (PHP_SAPI !== 'cli') {
    http_response_code(404);
    exit;
}

error_reporting(E_ALL);
set_error_handler(function ($severity, $message, $file, $line) {
    throw new ErrorException($message, 0, $severity, $file, $line);
});

$scenario = $argv[1] ?? 'duplicate';
if (!in_array($scenario, ['duplicate', 'registration', 'collision', 'getenv', 'filter_var', 'inet_pton', 'inet_ntop', 'logger'], true)) {
    throw new InvalidArgumentException('Unknown recovery scenario');
}
define('YOURLS_ABSPATH', dirname(__DIR__));
define('YOURLS_API', true);
$filters = [];
function yourls_add_filter($hook, $callback, $priority = 10) {
    if ($GLOBALS['scenario'] === 'registration') {
        throw new RuntimeException('private-registration-marker');
    }
    $GLOBALS['filters'][$hook][] = $callback;
}
if ($scenario === 'collision') {
    function appbox_middleware_normalize_ip($value) { return 'original-helper'; }
}
putenv('APPBOX_MIDDLEWARE_TRUSTED_IPS=192.0.2.10');
$plugin = dirname(__DIR__) . '/appbox-middleware-ip/plugin.php';
require $plugin;
require $plugin;
if (in_array($scenario, ['registration', 'collision'], true)) {
    if ($filters !== []) {
        throw new RuntimeException('Failed or conflicting registration changed the host hooks');
    }
} else {
    if (count($filters['get_IP'] ?? []) !== 1) {
        throw new RuntimeException('Duplicate inclusion registered extra callbacks');
    }
    $_SERVER = ['REMOTE_ADDR' => '192.0.2.10', 'HTTP_X_FORWARDED_FOR' => '198.51.100.7'];
    $actual = ($filters['get_IP'][0])('untrusted-original');
    $expected = in_array($scenario, ['getenv', 'logger'], true) ? '192.0.2.10' : ($scenario === 'duplicate' ? '198.51.100.7' : '');
    if ($actual !== $expected) {
        throw new RuntimeException('Attribution failure did not use a safe fallback');
    }
}
echo json_encode(['scenario' => $scenario, 'survived' => true]) . "\n";
