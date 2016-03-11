package gregor

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/jonboulle/clockwork"
	"strings"
	"time"
)

type SQLEngine struct {
	driver     *sql.DB
	objFactory ObjFactory
	clock      clockwork.Clock
}

func NewSQLEngine(d *sql.DB, of ObjFactory, cl clockwork.Clock) *SQLEngine {
	return &SQLEngine{driver: d, objFactory: of, clock: cl}
}

type queryBuilder struct {
	args  []interface{}
	qry   []string
	clock clockwork.Clock
}

func (q *queryBuilder) Build(f string, args ...interface{}) *queryBuilder {
	q.qry = append(q.qry, f)
	q.args = append(q.args, args...)
	return q
}

func (q *queryBuilder) Clone() *queryBuilder {
	ret := &queryBuilder{
		args: make([]interface{}, len(q.args)),
		qry:  make([]string, len(q.qry)),
	}
	copy(ret.args, q.args)
	copy(ret.qry, q.qry)
	return ret
}

func (q *queryBuilder) Query() string       { return strings.Join(q.qry, " ") }
func (q *queryBuilder) Args() []interface{} { return q.args }

func (q *queryBuilder) Exec(tx *sql.Tx) error {
	stmt, err := tx.Prepare(q.Query())
	if err != nil {
		return err
	}
	defer stmt.Close()
	fmt.Printf("Exec %q %+v\n", q.Query(), q.Args())
	_, err = stmt.Exec(q.Args()...)
	return err
}

func hexEnc(b byter) string { return hex.EncodeToString(b.Bytes()) }

func hexEncOrNull(b byter) interface{} {
	if b == nil {
		return nil
	}
	return hexEnc(b)
}

type byter interface {
	Bytes() []byte
}

func (q *queryBuilder) Now() *queryBuilder {
	if q.clock == nil {
		q.Build("NOW()")
	} else {
		q.Build("?", q.clock.Now())
	}
	return q
}

func (q *queryBuilder) TimeOrOffset(too TimeOrOffset) *queryBuilder {
	if too == nil {
		q.Build("NULL")
		return q
	}
	if t := too.Time(); t != nil {
		q.Build("?", *t)
		return q
	}
	if d := too.Duration(); d != nil {
		q.Build("DATE_ADD(")
		q.Now()
		q.Build(", INTERVAL ? MICROSECOND)", (d.Nanoseconds() / 1000))
		return q
	}
	q.Build("NULL")
	return q
}

func (s *SQLEngine) newQueryBuilder() *queryBuilder {
	return &queryBuilder{clock: s.clock}
}

func (s *SQLEngine) consumeCreation(tx *sql.Tx, u UID, i Item) error {
	md := i.Metadata()
	qb := s.newQueryBuilder()
	qb.Build("INSERT INTO items(uid, msgid, category, body, dtime) VALUES(?,?,?,?,",
		hexEnc(u),
		hexEnc(md.MsgID()),
		i.Category(),
		i.Body().Bytes(),
	)
	qb.TimeOrOffset(i.DTime())
	qb.Build(")")
	err := qb.Exec(tx)
	if err != nil {
		return err
	}

	for _, t := range i.NotifyTimes() {
		if t == nil {
			continue
		}
		nqb := s.newQueryBuilder()
		nqb.Build("INSERT INTO items(uid, msgid, ntime) VALUES(?,?,", hexEnc(u), hexEnc(md.MsgID()))
		nqb.TimeOrOffset(t)
		nqb.Build(")")
		err = nqb.Exec(tx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLEngine) consumeMsgIDsToDismiss(tx *sql.Tx, u UID, mid MsgID, dmids []MsgID) error {
	ins, err := tx.Prepare("INSERT INTO dismissals_by_id(uid, msgid, dmsgid) VALUES(?, ?, ?)")
	if err != nil {
		return err
	}
	defer ins.Close()
	qb := s.newQueryBuilder()
	qb.Build("UPDATE items SET dtime=")
	qb.Now()
	qb.Build("WHERE uid=? AND msgid=?")
	upd, err := tx.Prepare(qb.Query())
	if err != nil {
		return err
	}
	defer upd.Close()
	for _, dmid := range dmids {
		_, err = ins.Exec(hexEnc(u), hexEnc(mid), hexEnc(dmid))
		if err != nil {
			return err
		}
		qbc := qb.Clone()
		qbc.Build("", hexEnc(u), hexEnc(dmid))
		_, err = upd.Exec(qbc.Args()...)
		if err != nil {
			return err
		}
	}
	return err
}

func (s *SQLEngine) consumeRangesToDismiss(tx *sql.Tx, u UID, mid MsgID, mrs []MsgRange) error {
	for _, mr := range mrs {

		qb := s.newQueryBuilder()
		qb.Build("INSERT INTO dismissals_by_time(uid, msgid, category, dtime) VALUES(?,?,?,",
			hexEnc(u), hexEnc(mid), mr.Category().String())
		qb.TimeOrOffset(mr.EndTime())
		qb.Build(")")
		if err := qb.Exec(tx); err != nil {
			return err
		}

		qbu := s.newQueryBuilder()
		qbu.Build("UPDATE items SET dtime=")
		qbu.Now()
		qbu.Build("WHERE uid=? AND category=? AND ctime <= ",
			hexEnc(u), mr.Category().String())
		qbu.TimeOrOffset(mr.EndTime())
		if err := qbu.Exec(tx); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLEngine) consumeInbandMessageMetadata(tx *sql.Tx, md Metadata, t InbandMsgType) error {
	qb := s.newQueryBuilder()
	qb.Build("INSERT INTO messages(uid, msgid, mtype, ctime) VALUES(?, ?, ?,",
		hexEnc(md.UID()), hexEnc(md.MsgID()), int(t))
	qb.Now()
	qb.Build(")")
	return qb.Exec(tx)
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
	md := m.Metadata()
	if err := s.consumeInbandMessageMetadata(tx, md, InbandMsgTypeUpdate); err != nil {
		return err
	}
	fmt.Printf("A\n")
	if err := s.consumeCreation(tx, md.UID(), m.Creation()); err != nil {
		return err
	}
	fmt.Printf("B\n")
	if err := s.consumeMsgIDsToDismiss(tx, md.UID(), md.MsgID(), m.Dismissal().MsgIDsToDismiss()); err != nil {
		return err
	}
	fmt.Printf("C\n")
	if err := s.consumeRangesToDismiss(tx, md.UID(), md.MsgID(), m.Dismissal().RangesToDismiss()); err != nil {
		return err
	}
	fmt.Printf("D\n")

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLEngine) rowToItem(u UID, rows *sql.Rows) (Item, error) {
	var ctime time.Time
	deviceID := deviceIDScanner{o: s.objFactory}
	msgID := msgIDScanner{o: s.objFactory}
	category := categoryScanner{o: s.objFactory}
	body := bodyScanner{o: s.objFactory}
	var dtime timeOrNilScanner
	if err := rows.Scan(msgID, deviceID, category, dtime, body, &ctime); err != nil {
		return nil, err
	}
	return s.objFactory.MakeItem(u, msgID.MsgID(), deviceID.DeviceID(), ctime, category.Category(), dtime.Time(), body.Body())
}

func (s *SQLEngine) State(u UID, d DeviceID, t TimeOrOffset) (State, error) {
	items, err := s.items(u, d, t, nil)
	if err != nil {
		return nil, err
	}
	return s.objFactory.MakeState(items)
}

func (s *SQLEngine) items(u UID, d DeviceID, t TimeOrOffset, m MsgID) ([]Item, error) {
	qry := `SELECT i.msgid, m.devid, i.category, i.dtime, i.body, m.ctime
	        FROM items AS i
	        INNER JOIN messages AS m ON (i.uid=m.uid AND i.msgid=m.msgid)
	        WHERE i.dtime IS NULL AND i.uid=?`
	qb := s.newQueryBuilder()
	qb.Build(qry, hexEnc(u))
	if d != nil {
		qb.Build("AND i.devid=?", hexEnc(d))
	}
	if t != nil {
		qb.Build("AND m.ctime >=")
		qb.TimeOrOffset(t)
	}
	if m != nil {
		qb.Build("AND i.msgid=?", hexEnc(m))
	}
	qb.Build("ORDER BY m.ctime ASC")
	stmt, err := s.driver.Prepare(qb.Query())
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query(qb.Args()...)
	if err != nil {
		return nil, err
	}
	var items []Item
	for rows.Next() {
		item, err := s.rowToItem(u, rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *SQLEngine) rowToMetadata(rows *sql.Rows) (Metadata, error) {
	var ctime time.Time
	uid := uidScanner{o: s.objFactory}
	deviceID := deviceIDScanner{o: s.objFactory}
	msgID := msgIDScanner{o: s.objFactory}
	inbandMsgType := inbandMsgTypeScanner{}
	if err := rows.Scan(uid, msgID, &ctime, deviceID, inbandMsgType); err != nil {
		return nil, err
	}
	return s.objFactory.MakeMetadata(uid.UID(), msgID.MsgID(), deviceID.DeviceID(), ctime, inbandMsgType.InbandMsgType())
}

func (s *SQLEngine) inbandMetadataSince(u UID, t TimeOrOffset) ([]Metadata, error) {
	qry := `SELECT uid, msgid, ctime, devid, mtype FROM messages WHERE uid=?`
	qb := s.newQueryBuilder()
	qb.Build(qry, hexEnc(u))
	if t != nil {
		qb.Build("AND ctime >= ")
		qb.TimeOrOffset(t)
	}
	qb.Build("ORDER BY ctime ASC")
	stmt, err := s.driver.Prepare(qb.Query())
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query(qb.Args()...)
	if err != nil {
		return nil, err
	}
	var ret []Metadata
	for rows.Next() {
		md, err := s.rowToMetadata(rows)
		if err != nil {
			return nil, err
		}
		ret = append(ret, md)
	}
	return ret, nil
}

func (s *SQLEngine) rowToInbandMessage(u UID, rows *sql.Rows) (InbandMessage, error) {
	msgID := msgIDScanner{o: s.objFactory}
	devID := deviceIDScanner{o: s.objFactory}
	var ctime time.Time
	var mtype inbandMsgTypeScanner
	category := categoryScanner{o: s.objFactory}
	body := bodyScanner{o: s.objFactory}
	dCategory := categoryScanner{o: s.objFactory}
	var dTime timeOrNilScanner
	dMsgID := msgIDScanner{o: s.objFactory}

	if err := rows.Scan(msgID, devID, &ctime, mtype, category, body, dCategory, dTime, dMsgID); err != nil {
		return nil, err
	}

	switch {
	case category.IsSet():
		i, err := s.objFactory.MakeItem(u, msgID.MsgID(), devID.DeviceID(), ctime, category.Category(), nil, body.Body())
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInbandMessageFromItem(i)
	case dCategory.IsSet() && dTime.Time() != nil:
		d, err := s.objFactory.MakeDismissalByRange(u, msgID.MsgID(), devID.DeviceID(), ctime, dCategory.Category(), *(dTime.Time()))
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInbandMessageFromDismissal(d)
	case dMsgID.MsgID() != nil:
		d, err := s.objFactory.MakeDismissalByID(u, msgID.MsgID(), devID.DeviceID(), ctime, dMsgID.MsgID())
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInbandMessageFromDismissal(d)
	case mtype.InbandMsgType() == InbandMsgTypeSync:
		d, err := s.objFactory.MakeStateSyncMessage(u, msgID.MsgID(), devID.DeviceID(), ctime)
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInbandMessageFromStateSync(d)
	}

	return nil, nil
}

func (s *SQLEngine) InbandMessagesSince(u UID, d DeviceID, t TimeOrOffset) ([]InbandMessage, error) {
	qry := `SELECT m.msgid, m.devid, m.ctime, m.mtype,
               i.category, i.body,
               dt.category, dt.dtime,
               di.dmsgid
	        FROM messages AS m
	        LEFT JOIN items AS i ON (m.uid=i.UID AND m.msgid=i.msgid)
	        LEFT JOIN dismissals_by_time AS dt ON (m.uid=dt.uid AND m.msgid=dt.msgid)
	        LEFT JOIN dismissals_by_id AS di ON (m.uid=di.uid AND m.msgid=di.msgid)
	        WHERE ISNULL(i.dtime) AND i.uid=?`
	qb := s.newQueryBuilder()
	qb.Build(qry, hexEnc(u))
	if d != nil {
		qb.Build("AND i.devid=?", hexEnc(d))
	}

	qb.Build("AND m.ctime >= ")
	qb.TimeOrOffset(t)

	qb.Build("ORDER BY m.ctime ASC")
	stmt, err := s.driver.Prepare(qb.Query())
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	rows, err := stmt.Query(qb.Args()...)
	if err != nil {
		return nil, err
	}
	var ret []InbandMessage
	lookup := make(map[string]InbandMessage)
	for rows.Next() {
		ibm, err := s.rowToInbandMessage(u, rows)
		if err != nil {
			return nil, err
		}
		msgIDString := hexEnc(ibm.Metadata().MsgID())
		if ibm2 := lookup[msgIDString]; ibm2 != nil {
			if err = ibm2.Merge(ibm); err != nil {
				return nil, err
			}
		} else {
			ret = append(ret, ibm)
			lookup[msgIDString] = ibm
		}
	}
	return ret, nil
}

var _ StateMachine = (*SQLEngine)(nil)
