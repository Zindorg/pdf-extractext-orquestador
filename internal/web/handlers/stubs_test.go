package handlers

import (
	"context"

	"github.com/Zindorg/pdf-extractext-orquestador/internal/domain"
)

type extractStub struct {
	result domain.IngestResult
	err    error
	called bool
	in     domain.IncomingDocument
}

func (s *extractStub) Execute(ctx context.Context, in domain.IncomingDocument) (domain.IngestResult, error) {
	s.called = true
	s.in = in
	return s.result, s.err
}

type summaryStub struct {
	result domain.SummaryRequestResult
	err    error
}

func (s *summaryStub) Execute(ctx context.Context, id domain.DocumentID) (domain.SummaryRequestResult, error) {
	return s.result, s.err
}

type proxyStub struct {
	resp domain.PersisterResponse
	err  error
	req  domain.PersisterRequest
}

func (s *proxyStub) Forward(ctx context.Context, req domain.PersisterRequest) (domain.PersisterResponse, error) {
	s.req = req
	return s.resp, s.err
}
