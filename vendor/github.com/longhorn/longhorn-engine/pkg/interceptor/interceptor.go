package interceptor

import (
	context "context"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func WithIdentityValidationControllerServerInterceptor(volumeName, instanceName string) grpc.ServerOption {
	return grpc.UnaryInterceptor(identityValidationServerInterceptor(volumeName, instanceName, "controller"))
}

func WithIdentityValidationReplicaServerInterceptor(volumeName, instanceName string) grpc.ServerOption {
	return grpc.UnaryInterceptor(identityValidationServerInterceptor(volumeName, instanceName, "replica"))
}

func identityValidationServerInterceptor(volumeName, instanceName, serverType string) grpc.UnaryServerInterceptor {
	// Use a closure to remember the correct volumeName and/or instanceName.
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			var incomingVolumeName string
			incomingVolumeNames := md.Get("volume-name")
			if len(incomingVolumeNames) == 1 {
				// If len > 1, why? There is no legitimate reason, so do not validate.
				incomingVolumeName = incomingVolumeNames[0]
			}
			// Only refuse to serve if both client and server provide validation information.
			if incomingVolumeName != "" && volumeName != "" {
				log := logrus.WithFields(logrus.Fields{"method": info.FullMethod,
					"clientVolumeName": incomingVolumeName, "serverVolumeName": volumeName})
				if incomingVolumeName != volumeName {
					log.Error("Invalid gRPC metadata")
					return nil, status.Errorf(codes.FailedPrecondition, "incorrect volume name %s; check %s address",
						incomingVolumeName, serverType)
				}
				log.Trace("Valid gRPC metadata")
			}

			var incomingInstanceName string
			incomingInstanceNames := md.Get("instance-name")
			if len(incomingInstanceNames) == 1 {
				// If len > 1, why? There is no legitimate reason, so do not validate.
				incomingInstanceName = incomingInstanceNames[0]
			}
			// Only refuse to serve if both client and server provide validation information.
			if incomingInstanceName != "" && instanceName != "" {
				log := logrus.WithFields(logrus.Fields{"method": info.FullMethod,
					"clientInstanceName": incomingInstanceName, "serverInstanceName": instanceName})
				if incomingInstanceName != instanceName {
					log.Error("Invalid gRPC metadata")
					return nil, status.Errorf(codes.FailedPrecondition, "incorrect instance name %s; check %s address",
						incomingInstanceName, serverType)
				}
				log.Trace("Valid gRPC metadata")
			}
		}

		// Call the RPC's actual handler.
		return handler(ctx, req)
	}
}

func WithIdentityValidationClientInterceptor(volumeName, instanceName string) grpc.DialOption {
	return grpc.WithUnaryInterceptor(identityValidationClientInterceptor(volumeName, instanceName))
}

func identityValidationClientInterceptor(volumeName, instanceName string) grpc.UnaryClientInterceptor {
	// Use a closure to remember the correct volumeName and/or instanceName.
	return func(ctx context.Context, method string, req any, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if volumeName != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "volume-name", volumeName)
		}
		if instanceName != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "instance-name", instanceName)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
