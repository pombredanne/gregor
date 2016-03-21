package gregor

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/jonboulle/clockwork"
	"regexp"
	"strings"
	"time"
)

func sqlWrapper(s string) string {
	return strings.Join(regexp.MustCompile(`\s+`).Split(s, -1), " ")
}

type SQLEngine struct {
	driver     *sql.DB
	objFactory ObjFactory
	clock      clockwork.Clock
	stw        sqlTimeWriter
}

func NewSQLEngine(d *sql.DB, of ObjFactory, stw sqlTimeWriter, cl clockwork.Clock) *SQLEngine {
	return &SQLEngine{driver: d, objFactory: of, stw: stw, clock: cl}
}

type builder interface {
	Build(s string, args ...interface{})
}

type sqlTimeWriter interface {
	Now(b builder, cl clockwork.Clock)
	TimeOrOffset(b builder, cl clockwork.Clock, too TimeOrOffset)
}

type queryBuilder struct {
	args  []interface{}
	qry   []string
	clock clockwork.Clock
	stw   sqlTimeWriter
}

func (q *queryBuilder) Now() {
	q.stw.Now(q, q.clock)
}

func (q *queryBuilder) TimeOrOffset(too TimeOrOffset) {
	q.stw.TimeOrOffset(q, q.clock, too)
}

func (q *queryBuilder) Build(f string, args ...interface{}) {
	q.qry = append(q.qry, f)
	q.args = append(q.args, args...)
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

func (s *SQLEngine) newQueryBuilder() *queryBuilder {
	return &queryBuilder{clock: s.clock, stw: s.stw}
}

func (s *SQLEngine) consumeCreation(tx *sql.Tx, u UID, i Item) error {
	md := i.Metadata()
	qb := s.newQueryBuilder()
	qb.Build("INSERT INTO items(uid, msgid, category, body, dtime) VALUES(?,?,?,?,",
		hexEnc(u),
		hexEnc(md.MsgID()),
		i.Category().String(),
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
		qbu.Build("WHERE uid=? AND category=? AND msgid IN (SELECT msgid FROM messages WHERE uid=? AND ctime<=",
			hexEnc(u), mr.Category().String(), hexEnc(u))
		qbu.TimeOrOffset(mr.EndTime())
		qbu.Build(")")
		if err := qbu.Exec(tx); err != nil {
			return err
		}
	}
	return nil
}

func checkMetadataForInsert(m Metadata) error {
	if m.MsgID() == nil {
		return fmt.Errorf("bad metadata; nil MsgID")
	}
	if m.UID() == nil {
		return fmt.Errorf("bad metadata: nil UID")
	}
	return nil
}

func (s *SQLEngine) consumeInBandMessageMetadata(tx *sql.Tx, md Metadata, t InBandMsgType) error {
	if err := checkMetadataForInsert(md); err != nil {
		return err
	}
	if t != InBandMsgTypeUpdate && t != InBandMsgTypeSync {
		return fmt.Errorf("bad metadata: unrecognized msg type")
	}
	qb := s.newQueryBuilder()
	qb.Build("INSERT INTO messages(uid, msgid, mtype, devid, ctime) VALUES(?, ?, ?, ?,",
		hexEnc(md.UID()), hexEnc(md.MsgID()), int(t), hexEncOrNull(md.DeviceID()))
	qb.Now()
	qb.Build(")")
	return qb.Exec(tx)
}

func (s *SQLEngine) ConsumeMessage(m Message) error {
	switch {
	case m.ToInBandMessage() != nil:
		return s.consumeInBandMessage(m.ToInBandMessage())
	default:
		return nil
	}
}

func (s *SQLEngine) consumeInBandMessage(m InBandMessage) error {
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
	if err := s.consumeInBandMessageMetadata(tx, md, InBandMsgTypeUpdate); err != nil {
		return err
	}
	if m.Creation() != nil {
		if err := s.consumeCreation(tx, md.UID(), m.Creation()); err != nil {
			return err
		}
	}
	if m.Dismissal() != nil {
		if err := s.consumeMsgIDsToDismiss(tx, md.UID(), md.MsgID(), m.Dismissal().MsgIDsToDismiss()); err != nil {
			return err
		}
		if err := s.consumeRangesToDismiss(tx, md.UID(), md.MsgID(), m.Dismissal().RangesToDismiss()); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *SQLEngine) rowToItem(u UID, rows *sql.Rows) (Item, error) {
	deviceID := deviceIDScanner{o: s.objFactory}
	msgID := msgIDScanner{o: s.objFactory}
	category := categoryScanner{o: s.objFactory}
	body := bodyScanner{o: s.objFactory}
	var dtime timeScanner
	var ctime timeScanner
	if err := rows.Scan(&msgID, &deviceID, &category, &dtime, &body, &ctime); err != nil {
		return nil, err
	}
	return s.objFactory.MakeItem(u, msgID.MsgID(), deviceID.DeviceID(), ctime.Time(), category.Category(), dtime.TimeOrNil(), body.Body())
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
	        WHERE i.uid=? AND (i.dtime IS NULL OR i.dtime > `
	qb := s.newQueryBuilder()
	qb.Build(qry, hexEnc(u))
	if t != nil {
		qb.TimeOrOffset(t)
	} else {
		qb.Now()
	}
	qb.Build(")")
	if d != nil {
		qb.Build("AND m.devid=?", hexEnc(d))
	}
	if t != nil {
		qb.Build("AND m.ctime <=")
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
	inBandMsgType := inBandMsgTypeScanner{}
	if err := rows.Scan(&uid, &msgID, &ctime, &deviceID, &inBandMsgType); err != nil {
		return nil, err
	}
	return s.objFactory.MakeMetadata(uid.UID(), msgID.MsgID(), deviceID.DeviceID(), ctime, inBandMsgType.InBandMsgType())
}

func (s *SQLEngine) inBandMetadataSince(u UID, t TimeOrOffset) ([]Metadata, error) {
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

func (s *SQLEngine) rowToInBandMessage(u UID, rows *sql.Rows) (InBandMessage, error) {
	msgID := msgIDScanner{o: s.objFactory}
	devID := deviceIDScanner{o: s.objFactory}
	var ctime timeScanner
	var mtype inBandMsgTypeScanner
	category := categoryScanner{o: s.objFactory}
	body := bodyScanner{o: s.objFactory}
	dCategory := categoryScanner{o: s.objFactory}
	var dTime timeScanner
	dMsgID := msgIDScanner{o: s.objFactory}

	if err := rows.Scan(&msgID, &devID, &ctime, &mtype, &category, &body, &dCategory, &dTime, &dMsgID); err != nil {
		return nil, err
	}

	switch {
	case category.IsSet():
		i, err := s.objFactory.MakeItem(u, msgID.MsgID(), devID.DeviceID(), ctime.Time(), category.Category(), nil, body.Body())
		if err != nil {
			return nil, err
		}
		return s.objFactory.MakeInBandMessageFromItem(i)
	case dCategory.IsSet() && dTime.TimeOrNil() != nil:
		return s.objFactory.MakeDismissalByRange(u, msgID.MsgID(), devID.DeviceID(), ctime.Time(), dCategory.Category(), dTime.Time())
	case dMsgID.MsgID() != nil:
		return s.objFactory.MakeDismissalByID(u, msgID.MsgID(), devID.DeviceID(), ctime.Time(), dMsgID.MsgID())
	case mtype.InBandMsgType() == InBandMsgTypeSync:
		return s.objFactory.MakeStateSyncMessage(u, msgID.MsgID(), devID.DeviceID(), ctime.Time())
	}

	return nil, nil
}

func (s *SQLEngine) InBandMessagesSince(u UID, d DeviceID, t TimeOrOffset) ([]InBandMessage, error) {
	qry := `SELECT m.msgid, m.devid, m.ctime, m.mtype,
               i.category, i.body,
               dt.category, dt.dtime,
               di.dmsgid
	        FROM messages AS m
	        LEFT JOIN items AS i ON (m.uid=i.UID AND m.msgid=i.msgid)
	        LEFT JOIN dismissals_by_time AS dt ON (m.uid=dt.uid AND m.msgid=dt.msgid)
	        LEFT JOIN dismissals_by_id AS di ON (m.uid=di.uid AND m.msgid=di.msgid)
	        WHERE m.uid=? AND (i.dtime IS NULL OR i.dtime > `
	qb := s.newQueryBuilder()
	qb.Build(qry, hexEnc(u))
	qb.Now()
	qb.Build(")")
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
	var ret []InBandMessage
	lookup := make(map[string]InBandMessage)
	for rows.Next() {
		ibm, err := s.rowToInBandMessage(u, rows)
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
