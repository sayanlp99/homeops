-- A fridge note can take itself down: it shows through expires_on, then goes.
ALTER TABLE fridge_notes ADD COLUMN expires_on date;
