package chains

import "hexbridge/utils/msg"

type Router interface {
	Send(message msg.Message) error
}

//type Writer interface {
//	ResolveMessage(message msg.Message) bool
//}
