CREATE TABLE IF NOT EXISTS ocm_shares (
    id INT NOT NULL PRIMARY KEY AUTO_INCREMENT,
    token VARCHAR(255) NOT NULL UNIQUE,
    fileid_prefix VARCHAR(64) NOT NULL,
    item_source VARCHAR(64) NOT NULL,
    name TEXT NOT NULL,
    share_with VARCHAR(255) NOT NULL,
    owner VARCHAR(255) NOT NULL,
    initiator TEXT NOT NULL,
    ctime INTEGER NOT NULL,
    mtime INTEGER NOT NULL,
    expiration INTEGER DEFAULT NULL,
    type TINYINT NOT NULL,
    UNIQUE(fileid_prefix, item_source, share_with, owner)
);

CREATE TABLE IF NOT EXISTS ocm_shares_access_methods (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    ocm_share_id INTEGER NOT NULL,
    type TINYINT NOT NULL,
    FOREIGN KEY (ocm_share_id) REFERENCES ocm_shares(id) ON DELETE CASCADE,
    UNIQUE (ocm_share_id, type)
);

CREATE TABLE IF NOT EXISTS ocm_access_method_webdav (
    ocm_access_method_id INTEGER NOT NULL PRIMARY KEY,
    permissions INTEGER NOT NULL,
    FOREIGN KEY (ocm_access_method_id) REFERENCES ocm_shares_access_methods(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS ocm_access_method_webapp (
    ocm_access_method_id INTEGER NOT NULL PRIMARY KEY,
    view_mode INTEGER NOT NULL,
    FOREIGN KEY (ocm_access_method_id) REFERENCES ocm_shares_access_methods(id) ON DELETE CASCADE
);


CREATE TABLE IF NOT EXISTS ocm_received_shares (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    fileid_prefix VARCHAR(255) NOT NULL,
    item_source VARCHAR(255) NOT NULL,
    item_type TINYINT NOT NULL,
    share_with VARCHAR(255) NOT NULL,
    owner VARCHAR(255) NOT NULL,
    initiator VARCHAR(255) NOT NULL,
    ctime INTEGER NOT NULL,
    mtime INTEGER NOT NULL,
    expiration INTEGER DEFAULT NULL,
    type TINYINT NOT NULL,
    state TINYINT NOT NULL
);

CREATE TABLE IF NOT EXISTS ocm_received_share_protocols (
    id INTEGER NOT NULL PRIMARY KEY AUTO_INCREMENT,
    ocm_received_share_id INTEGER NOT NULL,
    type TINYINT NOT NULL,
    FOREIGN KEY (ocm_received_share_id) REFERENCES ocm_received_shares(id) ON DELETE CASCADE,
    UNIQUE (ocm_received_share_id, type)
);

CREATE TABLE IF NOT EXISTS ocm_protocol_webdav (
    ocm_protocol_id INTEGER NOT NULL PRIMARY KEY,
    uri VARCHAR(255) NOT NULL,
    shared_secret TEXT NOT NULL,
    permissions INTEGER NOT NULL,
    FOREIGN KEY (ocm_protocol_id) REFERENCES ocm_received_share_protocols(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS ocm_protocol_webapp (
    ocm_protocol_id INTEGER NOT NULL PRIMARY KEY,
    uri_template VARCHAR(255) NOT NULL,
    view_mode INTEGER NOT NULL,
    FOREIGN KEY (ocm_protocol_id) REFERENCES ocm_received_share_protocols(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS ocm_protocol_transfer (
    ocm_protocol_id INTEGER NOT NULL PRIMARY KEY,
    source_uri VARCHAR(255) NOT NULL,
    shared_secret VARCHAR(255) NOT NULL,
    size INTEGER NOT NULL,
    FOREIGN KEY (ocm_protocol_id) REFERENCES ocm_received_share_protocols(id) ON DELETE CASCADE
);
