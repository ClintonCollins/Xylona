package rpc

import (
	"context"

	connect_go "github.com/bufbuild/connect-go"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func (xs XylonaService) GameServerFilesDelete(ctx context.Context, request *connect_go.Request[xylona.GameServerFilesDeleteRequest]) (*connect_go.Response[xylona.GameServerFilesDeleteResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) GameServerFilesCompress(ctx context.Context, request *connect_go.Request[xylona.GameServerFilesCompressionRequest]) (*connect_go.Response[xylona.GameServerFilesCompressionResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) GameServerFilesDecompress(ctx context.Context, request *connect_go.Request[xylona.GameServerFilesDecompressionRequest]) (*connect_go.Response[xylona.GameServerFilesDecompressionResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) GameServerFilesDownloadFromURL(ctx context.Context, request *connect_go.Request[xylona.GameServersFileDownloadFromURLRequest]) (*connect_go.Response[xylona.GameServersFileDownloadFromURLResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) GameServerFileRename(ctx context.Context, request *connect_go.Request[xylona.GameServerFileRenameRequest]) (*connect_go.Response[xylona.GameServerFileRenameResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) GameServerFilesMove(ctx context.Context, request *connect_go.Request[xylona.GameServerFilesMoveRequest]) (*connect_go.Response[xylona.GameServerFilesMoveResponse], error) {
	//TODO implement me
	panic("implement me")
}

func (xs XylonaService) GameServersFileEdit(ctx context.Context, request *connect_go.Request[xylona.GameServersFileEditRequest]) (*connect_go.Response[xylona.GameServersFileEditResponse], error) {
	//TODO implement me
	panic("implement me")
}
