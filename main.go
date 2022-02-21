package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hyperledger/aries-framework-go/component/storageutil/mem"
	"github.com/hyperledger/aries-framework-go/pkg/client/didexchange"
	"github.com/hyperledger/aries-framework-go/pkg/client/mediator"
	"github.com/hyperledger/aries-framework-go/pkg/client/outofband"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/common/service"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/protocol/decorator"
	didsvc "github.com/hyperledger/aries-framework-go/pkg/didcomm/protocol/didexchange"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/transport"
	"github.com/hyperledger/aries-framework-go/pkg/didcomm/transport/ws"
	"github.com/hyperledger/aries-framework-go/pkg/framework/aries"
	"github.com/hyperledger/aries-framework-go/pkg/framework/context"
	"net/http"
)

func main() {

	didExchangeClient, ctx, err := createDIDClient(5000)
	if err != nil {
		panic(err)
	}

	http.Handle("/invitation", &InvitationHandler{DIDExchangeClient: *didExchangeClient, Provider: *ctx})
	http.ListenAndServe(":5000", nil)
}

type DIDExchangeClient struct {
	didexchange.Client
}

type InvitationHandler struct {
	DIDExchangeClient DIDExchangeClient
	Provider          context.Provider
}

func createDIDClient(port int32) (*DIDExchangeClient, *context.Provider, error) {

	ngrokAddress := fmt.Sprintf("localhost:%d", port+1)
	inbound, err := ws.NewInbound(ngrokAddress, "ws://"+ngrokAddress, "", "")
	if err != nil {
		panic(err)
	}

	framework, err := aries.New(
		aries.WithInboundTransport(inbound),
		aries.WithOutboundTransports(ws.NewOutbound()),
		aries.WithMediaTypeProfiles([]string{transport.MediaTypeDIDCommV2Profile}),
		//aries.WithKeyAgreementType(kms.NISTP384ECDHKWType), --> Leads to Error
		aries.WithStoreProvider(mem.NewProvider()),
		aries.WithProtocolStateStoreProvider(mem.NewProvider()),
	)
	if err != nil {
		panic(err)
	}

	ctx, err := framework.Context()
	if err != nil {
		panic("Failed to create framework context")
	}

	fmt.Println("Context created successfully")
	fmt.Println(ctx.ServiceEndpoint())

	// DID Exchange Client
	didExClient, err := didexchange.New(ctx)
	if err != nil {
		panic(err)
	}

	routerClient, err := mediator.New(ctx)
	if err != nil {
		panic(err)
	}

	go func() {
		handleDIDExchangeEvents(didExClient, routerClient)
	}()

	return &DIDExchangeClient{Client: *didExClient}, ctx, nil
}

func handleDIDExchangeEvents(didExClient *didexchange.Client, routerClient *mediator.Client) {

	events := make(chan service.DIDCommAction)
	err := didExClient.RegisterActionEvent(events)
	if err != nil {
		panic(err)
	}

	states := make(chan service.StateMsg)
	err = didExClient.RegisterMsgEvent(states)
	if err != nil {
		panic(err)
	}

	err = routerClient.RegisterActionEvent(events)
	if err != nil {
		panic(err)
	}

	err = routerClient.RegisterMsgEvent(states)
	if err != nil {
		panic(err)
	}

	//go func() {
	//	service.AutoExecuteActionEvent(events)
	//}()

	for {
		select {
		case event := <-events:

			fmt.Printf("Received %s\n", event.Message.Type())

			switch event.ProtocolName {

			case didexchange.ProtocolName:
				switch event.Message.Type() {
				case didexchange.RequestMsgType:
					req := &didsvc.Request{}
					err = event.Message.Decode(req)
					if err != nil {
						panic(err)
					}

					props, ok := event.Properties.(didexchange.Event)
					if !ok {
						panic("failed to cast event properties (shouldn't happen)")
					}

					fmt.Printf("Created connectionID %s\n", props.ConnectionID())
					event.Continue(nil)

				case didexchange.InvitationMsgType:
					event.Stop(errors.New("Rejected Didexchange Invitation"))

				}

			case mediator.ProtocolName:
				fmt.Println("Received a Mediator Event")
				if event.Message.Type() == mediator.RequestMsgType {
					event.Continue(nil)
				}

			}

		case state := <-states:
			fmt.Println(state.StateID)
			if state.StateID == "completed" {
				fmt.Println("Completed Connection")

			}
		}
	}

}

func (handler *InvitationHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:

		routeRequest := mediator.NewRequest()

		request, err := json.Marshal(routeRequest)
		if err != nil {
			panic(err)
		}
		fmt.Println(request)

		outOfBandClient, err := outofband.New(&handler.Provider)
		if err != nil {
			panic(err)
		}

		inv, err := outOfBandClient.CreateInvitation(
			nil,
			outofband.WithLabel("Router"),
			outofband.WithAttachments(&decorator.Attachment{
				Data: decorator.AttachmentData{
					//		JSON: routeRequest,
					Base64: base64.StdEncoding.EncodeToString(request),
				},
			}),
		)
		if err != nil {
			panic(err)
		}

		oobinv, err := json.Marshal(inv)
		if err != nil {
			panic(err)
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.Write(oobinv)
	}
}
