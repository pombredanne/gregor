// Auto-generated by avdl-compiler v1.3.3 (https://github.com/keybase/node-avdl-compiler)
//   Input file: gregor1/avdl/remind.avdl

package gregor1

import (
	rpc "github.com/keybase/go-framed-msgpack-rpc"
	context "golang.org/x/net/context"
)

type GetRemindersArg struct {
}

type DeleteRemindersArg struct {
	ReminderIDs []ReminderID `codec:"reminderIDs" json:"reminderIDs"`
}

type RemindInterface interface {
	// getReminders gets the reminders waiting to be sent out as a batch
	GetReminders(context.Context) (ReminderSet, error)
	// deleteReminders deletes all of the reminders by ReminderID
	DeleteReminders(context.Context, []ReminderID) error
}

func RemindProtocol(i RemindInterface) rpc.Protocol {
	return rpc.Protocol{
		Name: "gregor.1.remind",
		Methods: map[string]rpc.ServeHandlerDescription{
			"getReminders": {
				MakeArg: func() interface{} {
					ret := make([]GetRemindersArg, 1)
					return &ret
				},
				Handler: func(ctx context.Context, args interface{}) (ret interface{}, err error) {
					ret, err = i.GetReminders(ctx)
					return
				},
				MethodType: rpc.MethodCall,
			},
			"deleteReminders": {
				MakeArg: func() interface{} {
					ret := make([]DeleteRemindersArg, 1)
					return &ret
				},
				Handler: func(ctx context.Context, args interface{}) (ret interface{}, err error) {
					typedArgs, ok := args.(*[]DeleteRemindersArg)
					if !ok {
						err = rpc.NewTypeError((*[]DeleteRemindersArg)(nil), args)
						return
					}
					err = i.DeleteReminders(ctx, (*typedArgs)[0].ReminderIDs)
					return
				},
				MethodType: rpc.MethodCall,
			},
		},
	}
}

type RemindClient struct {
	Cli rpc.GenericClient
}

// getReminders gets the reminders waiting to be sent out as a batch
func (c RemindClient) GetReminders(ctx context.Context) (res ReminderSet, err error) {
	err = c.Cli.Call(ctx, "gregor.1.remind.getReminders", []interface{}{GetRemindersArg{}}, &res)
	return
}

// deleteReminders deletes all of the reminders by ReminderID
func (c RemindClient) DeleteReminders(ctx context.Context, reminderIDs []ReminderID) (err error) {
	__arg := DeleteRemindersArg{ReminderIDs: reminderIDs}
	err = c.Cli.Call(ctx, "gregor.1.remind.deleteReminders", []interface{}{__arg}, nil)
	return
}
