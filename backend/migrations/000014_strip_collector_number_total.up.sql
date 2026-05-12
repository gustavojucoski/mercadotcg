UPDATE cards
SET    collector_number = SPLIT_PART(collector_number, '/', 1),
       updated_at       = NOW()
WHERE  collector_number ~ '^\d+/\d+$';
