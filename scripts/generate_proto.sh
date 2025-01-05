# if there is error can ignore no folder exist, can ignore
echo "generating files, can ignore file not exist error"
rm -r types/protobuf/* 
protoc \
    --proto_path=types/proto \
    --go_out=types/protobuf \
    --go_opt=paths=source_relative \
    session/claims.proto \
    session/session.proto

echo "Done."
    

    



