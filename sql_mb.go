package gregor

import (
	"database/sql"
	"encoding/hex"
	"time"
)

type SQLEngine struct {
	driver     *sql.DB
	objFactory ObjFactory
}

func hexEnc(b getByter) string { return hex.EncodeToString(b.GetBytes()) }

type getByter interface {
	GetBytes() []byte
}

func timeOrOffsetToSQL(too TimeOrOffset) (string, interface{}) {
	if t := too.GetTime(); t != nil {
		return "?", *t
	}
	if d := too.GetDuration(); d != nil {
		return "DATE_ADD(NOW(), INTERVAL ? MICROSECOND)", d.Nanoseconds() / 1000
	}
	return "?", nil
}

func (s *SQLEngine) consumeCreation(tx *sql.Tx, u UID, i Item) error {
	q, a := timeOrOffsetToSQL(i.GetDTime())
	stmt, err := tx.Prepare(`
		INSERT INTO items(uid, msgid, devid, category, dtime, body)
		VALUES(?,?,?,?,` + q + `,?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	md := i.GetMetadata()
	_, err = stmt.Exec(hexEnc(u), hexEnc(md.GetMsgID()),
		hexEnc(md.GetDeviceID()), i.GetCategory(), a,
		i.GetBody().GetBytes())
	if err != nil {
		return err
	}

	for _, t := range i.GetNotifyTimes() {
		if t == nil {
			continue
		}
		q, a = timeOrOffsetToSQL(t)
		stmt, err = tx.Prepare(`
			INSERT INTO items(uid, msgid, ntime) VALUES(?, ?, ` + q + `)
		`)
		_, err = stmt.Exec(hexEnc(u), hexEnc(md.GetMsgID()), a)
		stmt.Close()
	}
	return nil
}

func (s *SQLEngine) consumeMsgIDsToDismiss(tx *sql.Tx, u UID, mid MsgID, dmids []MsgID) error {
	ins, err := tx.Prepare(`
		INSERT INTO dismissals_by_id(uid, msgid, dmsgid) VALUES(?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer ins.Close()
	upd, err := tx.Prepare(`
		UPDATE items SET dtime=NOW() WHERE uid=? AND msgid=?
	`)
	if err != nil {
		return err
	}
	defer upd.Close()
	for _, dmid := range dmids {
		_, err = ins.Exec(hexEnc(u), hexEnc(mid), hexEnc(dmid))
		if err != nil {
			return err
		}
		_, err = upd.Exec(hexEnc(u), hexEnc(dmid))
		if err != nil {
			return err
		}
	}
	return err
}

func (s *SQLEngine) consumeRangesToDismiss(tx *sql.Tx, u UID, mid MsgID, mrs []MsgRange) error {
	for _, mr := range mrs {
		q, a := timeOrOffsetToSQL(mr.GetEndTime())
		ins, err := tx.Prepare(`
			INSERT INTO dismissals_by_time(uid, msgid, category, dtime)
			VALUES(?,?,?,` + q + `)
		`)
		if err != nil {
			return err
		}
		defer ins.Close()
		_, err = ins.Exec(hexEnc(u), hexEnc(mid), mr.GetCategory().GetString(), a)
		if err != nil {
			return err
		}
		upd, err := tx.Prepare(`
			UPDATE items SET dtime=NOW() WHERE uid=? AND ctime<=` + q + ` AND category=?
		`)
		if err != nil {
			return err
		}
		defer upd.Close()
		_, err = upd.Exec(hexEnc(u), a, mr.GetCategory().GetString())
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLEngine) consumeInbandMessageMetadata(tx *sql.Tx, md Metadata) error {
	q, a := timeOrOffsetToSQL(md.GetCTime())
	ins, err := tx.Prepare(`
		INSERT INTO messages(uid, msgid, ctime)
		VALUES(?, ?, ` + q + `)
	`)
	if err != nil {
		return err
	}
	defer ins.Close()
	_, err = ins.Exec(hexEnc(md.GetUID()), hexEnc(md.GetMsgID()), a)
	return err
}

func (s *SQLEngine) ConsumeMessage(m Message) error {
	switch {
	case m.ToInbandMessage() != nil:
		return s.consumeInbandMessage(m.ToInbandMessage())
	default:
		return nil
	}
}

func (s *SQLEngine) consumeInbandMessage(m InbandMessage) error {
	switch {
	case m.ToStateUpdateMessage() != nil:
		return s.consumeStateUpdateMessage(m.ToStateUpdateMessage())
	default:
		return nil
	}
}

func (s *SQLEngine) consumeStateUpdateMessage(m StateUpdateMessage) error {
	tx, err := s.driver.Begin()
	if err != nil {
		return err
	}
	md := m.GetMetadata()
	if err := s.consumeInbandMessageMetadata(tx, md); err != nil {
		return err
	}
	if err := s.consumeCreation(tx, md.GetUID(), m.GetCreation()); err != nil {
		return err
	}
	if err := s.consumeMsgIDsToDismiss(tx, md.GetUID(), md.GetMsgID(), m.GetDismissal().GetMsgIDsToDismiss()); err != nil {
		return err
	}
	if err := s.consumeRangesToDismiss(tx, md.GetUID(), md.GetMsgID(), m.GetDismissal().GetRangesToDismiss()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLEngine) rowToItem(rows *sql.Rows) (Item, error) {
	var msgidHex, devidHex, category string
	var ctime, dtime time.Time
	var bodyBytes []byte
	if err := rows.Scan(&msgidHex, &devidHex, &category, &dtime, &bodyBytes, &ctime); err != nil {
		return nil, err
	}
	msgidBytes, err := hex.DecodeString(msgidHex)
	if err != nil {
		return nil, err
	}
	msgid, err := s.objFactory.MakeMsgID(msgidBytes)
	if err != nil {
		return nil, err
	}
	devidBytes, err := hex.DecodeString(msgidHex)
	if err != nil {
		return nil, err
	}
	devid, err := s.objFactory.MakeDeviceID(devidBytes)
	var dtimep *time.Time
	if !dtime.IsZero() {
		dtimep = &dtime
	}
	body, err := s.objFactory.MakeBody(bodyBytes)
	if err != nil {
		return nil, err
	}
	return s.objFactory.MakeItem(msgid, category, devid, ctime, dtimep, body)
}

func (s *SQLEngine) GetState(u UID, d DeviceID, t TimeOrOffset) (State, error) {
	items, err := s.getItems(u, d, t, nil)
	if err != nil {
		return nil, err
	}
	return s.objFactory.MakeState(items)
}

func (s *SQLEngine) getItems(u UID, d DeviceID, t TimeOrOffset, m MsgID) ([]Item, error) {
	qry := `SELECT i.msgid, m.devid, i.category, i.dtime, i.body, m.ctime
	        FROM items AS i
	        INNER JOIN messages AS m ON (i.uid=c.UID AND i.msgid=c.msgid)
	        WHERE ISNULL(i.dtime) AND i.uid=?`
	args := []interface{}{hexEnc(u)}
	if d != nil {
		qry += " AND i.devid=?"
		args = append(args, hexEnc(d))
	}
	if t != nil {
		q, a := timeOrOffsetToSQL(t)
		qry += " AND m.ctime=" + q
		args = append(args, a)
	}
	if m != nil {
		qry += " AND i.msgid=?"
		args = append(args, hexEnc(m))
	}
	qry += " ORDER BY m.ctime ASC"
	stmt, err := s.driver.Prepare(qry)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query(args...)
	var items []Item
	for rows.Next() {
		item, err := s.rowToItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *SQLEngine) getInbandMsgIDs(u UID, t TimeOrOffset) ([]MsgID, error) {
	return nil, nil
}

func (s *SQLEngine) GetInbandMessagesSince(u UID, d DeviceID, t TimeOrOffset) ([]InbandMessage, error) {
	_, err := s.getInbandMsgIDs(u, t)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

var _ StateMachine = (*SQLEngine)(nil)
