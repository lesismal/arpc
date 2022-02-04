package util

import (
	"reflect"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
)

func Bind(handler arpc.Handler, service interface{}) {
	serviceType := reflect.TypeOf(service)

	for i := 0; i < serviceType.NumMethod(); i++ {
		method := serviceType.Method(i)
		if !method.IsExported() {
			continue
		}

		handler.Handle(method.Name, func(context *arpc.Context) {
			argsSize := method.Type.NumIn()
			args := make([]reflect.Value, argsSize)
			for i := 0; i < argsSize; i++ {
				argType := method.Type.In(i)
				switch argType {
				case serviceType:
					args[i] = reflect.ValueOf(service)
					break
				case reflect.TypeOf(new(arpc.Client)):
					args[i] = reflect.ValueOf(context.Client)
				default:
					var arg reflect.Value
					if argType.Kind() == reflect.Ptr {
						arg = reflect.New(argType.Elem())
					} else {
						arg = reflect.New(argType)
					}

					if err := context.Bind(arg.Interface()); err != nil {
						log.Error("%v %v \naddr = %v \nmethod = %s \ncodec = %v", context.Client.Handler.LogTag(), err, context.Client.Conn.RemoteAddr(), method.Name, reflect.TypeOf(context.Client.Codec))
						if err = context.Error(err); err != nil {
							log.Error("%v %v \naddr = %v \nmethod = %s", context.Client.Handler.LogTag(), err, context.Client.Conn.RemoteAddr(), method.Name)
						}
					}

					if argType.Kind() == reflect.Ptr {
						args[i] = arg
					} else {
						args[i] = arg.Elem()
					}
				}
			}

			defer func() {
				if err := recover(); err != nil {
					log.Error("%v %v \naddr = %v \nmethod = %s", context.Client.Handler.LogTag(), err, context.Client.Conn.RemoteAddr(), method.Name)
					if err = context.Error(err); err != nil {
						log.Error("%v %v \naddr = %v \nmethod = %s", context.Client.Handler.LogTag(), err, context.Client.Conn.RemoteAddr(), method.Name)
					}

				}
			}()

			args = method.Func.Call(args)
			//if args [ 0 ].IsNil () {
			//	log.Error ( "%v %v \naddr = %v \nmethod = %s", context.remoteClient.Handler.LogTag (), args [ 1 ].Interface ().( error ), context.remoteClient.Conn.RemoteAddr (), method.Name )
			//	context.Error ( args [ 1 ].Interface ().( error ) )
			//}

			if len(args) == 0 {
				return
			}

			if err := context.Write(args[0].Interface()); err != nil {
				log.Error("%v %v \naddr = %v \nmethod = %s", context.Client.Handler.LogTag(), err, context.Client.Conn.RemoteAddr(), method.Name)
			}
		})
	}
}
