#!/usr/bin/env bash

cp /tls/*.crt /usr/local/share/ca-certificates/
update-ca-certificates

php console.php maintenance:install --admin-user "${USER}" --admin-pass "${PASS}" --database "mysql"            \
                                    --database-name "efss" --database-user "root" --database-host "$DBHOST"     \
                                    --database-pass "eilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek"
php console.php app:disable firstrunwizard

sed -i "8 i\      1 => 'owncloud1.docker'," /var/www/html/config/config.php
sed -i "9 i\      2 => 'owncloud2.docker'," /var/www/html/config/config.php

php console.php app:enable sciencemesh
sed -i "3 i\  'sharing.managerFactory' => 'OCA\\\\ScienceMesh\\\\ScienceMeshProviderFactory'," /var/www/html/config/config.php
sed -i "4 i\  'sharing.remoteShareesSearch' => 'OCA\\\\ScienceMesh\\\\Plugins\\\\ScienceMeshSearchPlugin'," /var/www/html/config/config.php

