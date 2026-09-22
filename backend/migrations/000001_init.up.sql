CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username text NOT NULL,
  display_name text NOT NULL,
  status text,
  password_hash text NOT NULL,
  avatar_media_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_username_lower_idx ON users (lower(username));
CREATE TABLE app_settings (singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton), registration_enabled boolean NOT NULL DEFAULT true);
INSERT INTO app_settings(singleton, registration_enabled) VALUES (true, true) ON CONFLICT DO NOTHING;
CREATE TABLE sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE, expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz, ip_hash text, user_agent text
);
CREATE TABLE storage_spaces (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL, description text, address text, owner_user_id uuid NOT NULL REFERENCES users(id), archived_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE memberships (storage_space_id uuid NOT NULL REFERENCES storage_spaces(id) ON DELETE CASCADE, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, role text NOT NULL CHECK(role IN ('owner','admin','editor','viewer')), joined_at timestamptz NOT NULL DEFAULT now(), left_at timestamptz, PRIMARY KEY(storage_space_id,user_id));
CREATE TABLE media (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), storage_space_id uuid NOT NULL REFERENCES storage_spaces(id) ON DELETE CASCADE, object_key text NOT NULL UNIQUE, mime_type text NOT NULL, byte_size bigint NOT NULL, created_by uuid NOT NULL REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
CREATE TABLE locations (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), storage_space_id uuid NOT NULL REFERENCES storage_spaces(id) ON DELETE CASCADE, parent_location_id uuid REFERENCES locations(id), name text NOT NULL, code text, description text, icon text, state text NOT NULL DEFAULT 'active', deleted_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(storage_space_id, code));
CREATE TABLE boxes (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), storage_space_id uuid NOT NULL REFERENCES storage_spaces(id) ON DELETE CASCADE, name text NOT NULL, description text, current_location_id uuid REFERENCES locations(id), temporary_location_id uuid REFERENCES locations(id), temporary_until timestamptz, state text NOT NULL DEFAULT 'active', deleted_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE items (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), storage_space_id uuid NOT NULL REFERENCES storage_spaces(id) ON DELETE CASCADE, name text NOT NULL, description text, box_id uuid REFERENCES boxes(id), state text NOT NULL DEFAULT 'active', deleted_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce(name,'') || ' ' || coalesce(description,''))) STORED);
CREATE INDEX items_search_idx ON items USING GIN(search_vector); CREATE INDEX items_name_trgm_idx ON items USING GIN(name gin_trgm_ops);
CREATE TABLE item_media (item_id uuid NOT NULL REFERENCES items(id) ON DELETE CASCADE, media_id uuid NOT NULL REFERENCES media(id), position int NOT NULL DEFAULT 0, is_cover boolean NOT NULL DEFAULT false, PRIMARY KEY(item_id,media_id));
CREATE TABLE tags (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), storage_space_id uuid NOT NULL REFERENCES storage_spaces(id) ON DELETE CASCADE, name text NOT NULL, color text, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(storage_space_id,name));
CREATE TABLE item_tags (item_id uuid NOT NULL REFERENCES items(id) ON DELETE CASCADE, tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY(item_id,tag_id));
CREATE TABLE audit_events (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), storage_space_id uuid NOT NULL REFERENCES storage_spaces(id) ON DELETE CASCADE, entity_type text NOT NULL, entity_id uuid NOT NULL, action text NOT NULL, actor_user_id uuid REFERENCES users(id), payload jsonb NOT NULL DEFAULT '{}', occurred_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX audit_events_entity_idx ON audit_events(entity_type,entity_id,occurred_at DESC);
