CREATE TABLE media_conversion_jobs (
  media_id uuid PRIMARY KEY REFERENCES media(id) ON DELETE CASCADE,
  source_object_key text,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
  attempts integer NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  locked_at timestamptz,
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);
CREATE INDEX media_conversion_jobs_pending_idx ON media_conversion_jobs(status, next_attempt_at);

INSERT INTO media_conversion_jobs (media_id)
SELECT id FROM media WHERE deleted_at IS NULL
ON CONFLICT (media_id) DO NOTHING;
