
CREATE TABLE `items` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`category` VARCHAR(128) NOT NULL,
	`dtime` DATETIME(6),
	`body` BLOB,
	FOREIGN KEY(`uid`, `msgid`) REFERENCES `messages` (`uid`, `msgid`),
	PRIMARY KEY(`uid`, `msgid`),
	KEY `user_order` (`uid`, `category`, `ctime`),
	KEY `cleanup_order` (`uid`, `dtime`)
);

CREATE TABLE `messages` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`ctime` DATETIME(6) NOT NULL,
	`devid` CHAR(16),
	`mtype` INTEGER UNSIGNED NOT NULL COMMENT "specify for 'Update' or 'Sync' types",
	PRIMARY KEY(`uid`, `msgid`)
);

CREATE TABLE `reminders` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`ntime` DATETIME(6) NOT NULL,
	PRIMARY KEY(`uid`, `msgid`, `ntime`)
);

CREATE TABLE `dismissals_by_id` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`dmsgid` CHAR(16) NOT NULL COMMENT "the message IDs to dismiss",
	FOREIGN KEY(`uid`, `msgid`) REFERENCES `messages` (`uid`, `msgid`),
	PRIMARY KEY(`uid`, `msgid`, `dmsgid`)
);

CREATE TABLE `dismissals_by_time` (
	`uid`   CHAR(16) NOT NULL,
	`msgid` CHAR(16) NOT NULL,
	`category` VARCHAR(128) NOT NULL,
	`dtime` DATETIME(6) NOT NULL COMMENT "throw out matching events before dtime",
	FOREIGN KEY(`uid`, `msgid`) REFERENCES `messages` (`uid`, `msgid`),
	PRIMARY KEY(`uid`, `msgid`, `category`, `dtime`),
);