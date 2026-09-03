package main

import (
	"log"

	"golang.org/x/sys/windows/svc"
)

type tubiService struct{}

func (m *tubiService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	go runInteractive()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			sigMu.Lock()
			if cancelCtx != nil {
				cancelCtx()
			}
			sigMu.Unlock()
			return false, 0
		}
	}
	return false, 0
}

func runService(name string) {
	err := svc.Run(name, &tubiService{})
	if err != nil {
		log.Fatalf("Windows service %s failed: %v", name, err)
	}
}