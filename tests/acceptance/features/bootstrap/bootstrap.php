<?php
$pathToApiTests = \getenv('PATH_TO_APITESTS');
if ($pathToApiTests === false) {
    $pathToApiTests = "../ocis";
}

require_once $pathToApiTests . '/tests/acceptance/features/bootstrap/bootstrap.php';

$classLoader = new \Composer\Autoload\ClassLoader();
$classLoader->addPsr4(
	"", $pathToApiTests . "/tests/acceptance/features/bootstrap", true
);

$classLoader->register();
