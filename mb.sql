
CREATE TABLE `items` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`devid` CHAR(16),
	`category` VARCHAR(128) NOT NULL,
	`dtime` DATETIME,
	`body` BLOB,
	PRIMARY KEY(`uid`, `msgid`),
	FOREIGN KEY(`uid`, `msgid`) REFERENCES `messages` (`uid`, `msgid`),
	KEY `user_order` (`uid`, `category`, `ctime`),
	KEY `cleanup_order` (`uid`, `dtime`)
);

CREATE TABLE `messages` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`ctime` DATETIME NOT NULL,
	PRIMARY KEY(`uid`, `msgid`)
);

CREATE TABLE `reminders` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`ntime` DATETIME NOT NULL,
	PRIMARY KEY(`uid`, `msgid`, `ntime`),
);

CREATE TABLE `dismissals_by_id` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`dmsgid` CHAR(16) NOT NULL COMMENT "the message IDs to dismiss",
	PRIMARY KEY(`uid`, `msgid`, `dmsgid`),
	FOREIGN KEY(`uid`, `msgid`) REFERENCES `messages` (`uid`, `msgid`)
);

CREATE TABLE `dismissals_by_time` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`category` VARCHAR(128) NOT NULL,
	`dtime` DATETIME NOT NULL COMMENT "throw out matching events before dtime",
	PRIMARY KEY(`uid`, `msgid`, `category`, `dtime`),
	FOREIGN KEY(`uid`, `msgid`) REFERENCES `messages` (`uid`, `msgid`)
);