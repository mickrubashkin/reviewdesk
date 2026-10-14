-- +goose Up
CREATE TABLE auth_otp_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL,
  code_hash TEXT NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX auth_otp_codes_email_created_at_idx ON auth_otp_codes (email, created_at DESC);

-- +goose Down
DROP TABLE auth_otp_codes;
