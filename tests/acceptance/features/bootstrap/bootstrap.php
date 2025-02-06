<?php
require_once '../opencloud/tests/acceptance/bootstrap/bootstrap.php';

$classLoader = new \Composer\Autoload\ClassLoader();
$classLoader->addPsr4(
	"", "../opencloud/tests/acceptance/bootstrap", true
);

$classLoader->register();
