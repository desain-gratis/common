# if there is error can ignore no folder exist, can ignore
echo "generating files, can ignore file not exist error"
rm -r types/protobuf/* 
~/bin/protoc-3.19.1/bin/protoc \
    --proto_path=types/idl \
    --go_out=types/protobuf \
    --go_opt=paths=source_relative \
    contentupload/attachment.proto \
    contentupload/contentupload.proto \
    contentupload/file.proto \
    contentupload/image.proto \
    session/claims.proto \
    session/session.proto \
    service/auth-api/auth.proto \
    --go-grpc_out=types/protobuf \
    --go-grpc_opt=paths=source_relative    

echo "Done."
    

    



