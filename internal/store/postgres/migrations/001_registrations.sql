CREATE TABLE registration_gate (
    singleton_id smallint PRIMARY KEY CHECK (singleton_id = 1),
    used bigint NOT NULL CHECK (used >= 0),
    max_registrations bigint NOT NULL CHECK (max_registrations > 0),
    CHECK (used <= max_registrations)
);

CREATE TABLE machine_registrations (
    request_id uuid PRIMARY KEY,
    machine_id uuid NOT NULL UNIQUE,
    name text NOT NULL UNIQUE
        CHECK (octet_length(name) BETWEEN 1 AND 63)
        CHECK (name ~ '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'),
    image text NOT NULL
        CHECK (octet_length(image) BETWEEN 1 AND 128)
        CHECK (image ~ '^[A-Za-z0-9._-]+$'),
    vcpus integer NOT NULL CHECK (vcpus BETWEEN 1 AND 64),
    memory_mib bigint NOT NULL CHECK (memory_mib BETWEEN 128 AND 1048576),
    disk_mib bigint NOT NULL CHECK (disk_mib BETWEEN 1024 AND 16777216),
    fingerprint_version smallint NOT NULL CHECK (fingerprint_version = 1),
    fingerprint bytea NOT NULL CHECK (octet_length(fingerprint) = 32),
    state text NOT NULL CHECK (state = 'registered'),
    created_at timestamptz NOT NULL DEFAULT now()
);
