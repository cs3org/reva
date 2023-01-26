<?php
require_once '../ocis/tests/acceptance/features/bootstrap/bootstrap.php';

$classLoader = new \Composer\Autoload\ClassLoader();
$classLoader->addPsr4(
	"", "../ocis/tests/acceptance/features/bootstrap", true
);

$classLoader->register();
