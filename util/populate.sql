-- insert a bunch of random message rows into gregor db
DELIMITER $$
CREATE PROCEDURE insert_random_data()
BEGIN
  DECLARE i INT DEFAULT 0;

  WHILE i < 100000 DO
    INSERT INTO messages (uid, msgid, ctime, devid, mtype) VALUES (MD5(RAND()), MD5(RAND()), NOW(), MD5(RAND()), 1);
    SET i = i + 1;
  END WHILE;
END$$
DELIMITER ;


CALL insert_random_data();

DROP PROCEDURE insert_random_data;
