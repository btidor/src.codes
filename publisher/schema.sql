-- The `package_versions` table tracks the unique packages in our system. Every
-- version of every package is a row in this table. (If the same version of the
-- same package appears in different distributions, each distro gets its own row
-- too.)
CREATE TABLE package_versions (
    id              SERIAL NOT NULL,

    distro          VARCHAR(32) NOT NULL,
    pkg_name        VARCHAR(255) NOT NULL,
    pkg_version     VARCHAR(255) NOT NULL,

    -- The epoch represents which version of the archiver last processed the
    -- package. (It's also included in the index filenames on `ls` and `meta`).
    -- If the archiver is updated to produce new indexes or formats, we'll
    -- reprocess old packages and bump their epoch.
    sc_epoch        INT NOT NULL,

    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX package_version
ON package_versions (distro, pkg_name, pkg_version);

-- The `distribution_contents` table mirrors the contents of the Sources file
-- published by each distribution. It lists the latest version of each package
-- included in the distribution, and excludes any packages that have been
-- removed from the distribution. This table gets re-written as packages are
-- updated, added and removed.
CREATE TABLE distribution_contents (
    id              SERIAL NOT NULL,

    distro          VARCHAR(32) NOT NULL,
    pkg_name        VARCHAR(255) NOT NULL,
    current         INT NOT NULL, -- foreign key to package_versions

    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX package on distribution_contents (distro, pkg_name);

-- The `files` table tracks the files uploaded to B2. We check this table to
-- avoid uploading the same file multiple times. This table takes up the vast
-- majority of our database storage, so we avoid storing any other columns, and
-- we truncate file hashes to 64 bits. (Files are stored under the full SHA-256
-- hash, so if two files have a collision in the first 64 bits, we'll skip
-- uploading one of the two and requests to retrieve it will 404. Sorry!)
CREATE TABLE files (
    short_hash      BIGINT NOT NULL,
    PRIMARY KEY (short_hash)
);
