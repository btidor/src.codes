CREATE TABLE packages (
    -- Information about the distribution:
    --  * distro: hirsute, bullseye, etc.
    --  * area: NULL, backports, proposed, etc.
    --  * component: main, multiverse, etc.
    distribution    VARCHAR(32) NOT NULL,
    area            VARCHAR(32),
    component       VARCHAR(32),

    -- Package Info
    package_name    VARCHAR(255) NOT NULL,
    package_version VARCHAR(255) NOT NULL,

    -- SHA-256 hash of the source control file (*.dsc). If multiple packages
    -- have the same control hash, we only process it once.
    control_hash    CHAR(64) NOT NULL,

    -- Short prefix of the SHA-256 hash of the src.codes index file (*.json).
    -- Used in the file name for cacheability.
    index_slug      CHAR(8) NOT NULL,

    processed_at    TIMESTAMP DEFAULT UTC_TIMESTAMP(),

    PRIMARY KEY (distribution, package_name, package_version),
    INDEX hash_lookup (control_hash)
);

CREATE TABLE latest_packages (
    distribution    VARCHAR(32) NOT NULL,
    package_name    VARCHAR(255) NOT NULL,
    latest_version  VARCHAR(255) NOT NULL,

    PRIMARY KEY (distribution, package_name)
);

CREATE TABLE files (
    -- Identifies the package(s) in which this file appears from the 'packages'
    -- table.
    control_hash   CHAR(64) NOT NULL,

    -- Path to the file in the source archive, relative to the archive root.
    file_path       VARCHAR(511) NOT NULL,

    file_hash       CHAR(64) NOT NULL,
    file_size       BIGINT,

    PRIMARY KEY (control_hash, file_path),
    INDEX hash_lookup (file_hash)
);
