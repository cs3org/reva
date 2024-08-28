<?php
require_once '../ocis/tests/acceptance/bootstrap/bootstrap.php';

$classLoader = new \Composer\Autoload\ClassLoader();
$classLoader->addPsr4(
	"", "../ocis/tests/acceptance/bootstrap", true
);

$classLoader->register();
