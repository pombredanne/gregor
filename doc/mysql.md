Testing delete on InnoDB tables
-------------------------------

Curious if deleting data from InnoDB tables reclaims
space, or if table data file grows without bounds.

1. Using `util/populate.sql`, added 1M messages.
2. `messages.ibd` after is 236MB.
3. `DELETE FROM messages;` to remove all the rows.
4. `messages.ibd` after is 236MB (as expected).
5. Added 100k rows, `messages.ibd` still 236MB.
6. Added 1M rows (total), `messages.ibd` still 236MB.
7. Added 1.1M rows (total), `messages.ibd` now 252MB.

Conclusion: while the InnoDB file size doesn't shrink
after deleting rows, the space that was used by deleted 
rows is reused for new rows.

Tested on 5.7.10 on OS X and 5.6.28 on linux, same
results.
