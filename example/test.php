<?php

$redis_handle = new Redis();
$redis_handle->connect('127.0.0.1', 8399, 30);
$redis_handle->rawCommand('listfiles', '/web');
