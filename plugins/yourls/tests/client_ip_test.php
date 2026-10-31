<?php

if (PHP_SAPI !== 'cli') {
    http_response_code(404);
    exit;
}

error_reporting(E_ALL);
set_error_handler(function ($severity, $message, $file, $line) {
    throw new ErrorException($message, 0, $severity, $file, $line);
});

$mode = $argv[1] ?? 'api';
if (!in_array($mode, ['api', 'web', 'undefined'], true)) {
    throw new InvalidArgumentException('Expected api, web or undefined');
}
define('YOURLS_ABSPATH', dirname(__DIR__));
if ($mode !== 'undefined') {
    define('YOURLS_API', $mode === 'api');
}

$filters = [];
function yourls_add_filter($hook, $callback, $priority = 10) {
    $GLOBALS['filters'][$hook] = [$callback, $priority];
}

require dirname(__DIR__) . '/appbox-middleware-ip/plugin.php';
if (($filters['get_IP'] ?? null) !== ['appbox_middleware_client_ip', 99]) {
    throw new RuntimeException('Plugin did not register the expected filter');
}
$callback = $filters['get_IP'][0];
$trusted = '192.0.2.10,2001:db8::10';
$cases = [
    ['unset trust', null, '192.0.2.10', '198.51.100.7', '192.0.2.10'],
    ['empty trust', '', '192.0.2.10', '198.51.100.7', '192.0.2.10'],
    ['empty entries', ' , , ', '192.0.2.10', '198.51.100.7', '192.0.2.10'],
    ['invalid trust', 'invalid', '192.0.2.10', '198.51.100.7', '192.0.2.10'],
    ['subnet is not an exact peer', '192.0.2.0/24', '192.0.2.10', '198.51.100.7', '192.0.2.10'],
    ['trusted IPv4', $trusted, '192.0.2.10', '198.51.100.7', '198.51.100.7'],
    ['trusted IPv6', $trusted, '2001:db8::10', '2001:db8::7', '2001:db8::7'],
    ['canonical peer', $trusted, '2001:0db8:0:0::10', '198.51.100.7', '198.51.100.7'],
    ['canonical configuration', '2001:0db8:0:0::10', '2001:db8::10', '198.51.100.7', '198.51.100.7'],
    ['mapped peer', $trusted, '::ffff:192.0.2.10', '198.51.100.7', '198.51.100.7'],
    ['mapped configuration', '::ffff:192.0.2.10', '192.0.2.10', '198.51.100.7', '198.51.100.7'],
    ['canonical client', $trusted, '192.0.2.10', '2001:0db8:0:0::7', '2001:db8::7'],
    ['mapped client', $trusted, '192.0.2.10', '::ffff:198.51.100.7', '198.51.100.7'],
    ['untrusted IPv4', $trusted, '192.0.2.20', '198.51.100.7', '192.0.2.20'],
    ['untrusted IPv6', $trusted, '2001:db8::20', '198.51.100.7', '2001:db8::20'],
    ['mixed configuration', 'invalid, , 192.0.2.10 ', '192.0.2.10', '198.51.100.7', '198.51.100.7'],
    ['missing header', $trusted, '192.0.2.10', '', '192.0.2.10'],
    ['invalid header', $trusted, '192.0.2.10', 'not-an-ip', '192.0.2.10'],
    ['multiple addresses', $trusted, '192.0.2.10', '203.0.113.99,198.51.100.7', '192.0.2.10'],
    ['trailing comma', $trusted, '192.0.2.10', '198.51.100.7,', '192.0.2.10'],
    ['scoped IPv6', $trusted, '192.0.2.10', 'fe80::7%eth0', '192.0.2.10'],
    ['address with port', $trusted, '192.0.2.10', '198.51.100.7:80', '192.0.2.10'],
    ['newline', $trusted, '192.0.2.10', "198.51.100.7\r\n", '192.0.2.10'],
    ['spaces', $trusted, '192.0.2.10', ' 198.51.100.7 ', '198.51.100.7'],
    ['nonstring header', $trusted, '192.0.2.10', ['198.51.100.7'], '192.0.2.10'],
    ['invalid peer', $trusted, 'not-an-ip', '198.51.100.7', ''],
    ['missing peer', $trusted, '', '198.51.100.7', ''],
];

$count = 0;
foreach ($cases as [$name, $configuration, $peer, $forwarded, $expected]) {
    putenv($configuration === null ? 'APPBOX_MIDDLEWARE_TRUSTED_IPS' : 'APPBOX_MIDDLEWARE_TRUSTED_IPS=' . $configuration);
    $_SERVER = ['REMOTE_ADDR' => $peer, 'HTTP_X_FORWARDED_FOR' => $forwarded];
    if ($mode !== 'api') {
        $expected = 'core-ip';
    }
    $actual = $callback('core-ip');
    if ($actual !== $expected) {
        throw new RuntimeException("$name: got " . json_encode($actual) . ', expected ' . json_encode($expected));
    }
    $count++;
}
putenv('APPBOX_MIDDLEWARE_TRUSTED_IPS=' . $trusted);
$_SERVER = ['REMOTE_ADDR' => '192.0.2.10', 'HTTP_CLIENT_IP' => '203.0.113.99', 'HTTP_VIA' => '203.0.113.98', 'HTTP_CF_CONNECTING_IP' => '203.0.113.97'];
if ($callback('core-ip') !== ($mode === 'api' ? '192.0.2.10' : 'core-ip')) {
    throw new RuntimeException('Secondary IP headers changed the result');
}
echo json_encode(['mode' => $mode, 'checks' => $count + 1]) . "\n";
