# pages

Pages stores pages of blob data. It is essentially a minimal version of
BoltDB, where the the B+ tree was removed and replaced by simply writing
page-aligned byte slices.
