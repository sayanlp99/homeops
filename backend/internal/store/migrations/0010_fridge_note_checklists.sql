-- A fridge note can be a checklist: items [{id, text, done}], ticked off right on the
-- note. A checklist's text is its optional title, so text may now be empty.
ALTER TABLE fridge_notes ADD COLUMN items jsonb NOT NULL DEFAULT '[]';
ALTER TABLE fridge_notes DROP CONSTRAINT fridge_notes_text_check;
ALTER TABLE fridge_notes ADD CONSTRAINT fridge_notes_text_check CHECK (length(text) <= 500);
