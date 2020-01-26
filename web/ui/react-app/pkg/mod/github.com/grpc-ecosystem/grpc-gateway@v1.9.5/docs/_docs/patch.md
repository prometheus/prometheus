---
category: documentation
---

# Patch Feature
The HTTP PATCH method allows a resource to be partially updated. 

The idea, If a binding is mapped to patch and the request message has exactly one FieldMask message in it, additional code is rendered for the gateway handler that will populate the FieldMask based on the request body.
This handles two scenarios: 
- The FieldMask is hidden from the REST request (as in the [UpdateV2](https://github.com/grpc-ecosystem/grpc-gateway/blob/master/examples/proto/examplepb/a_bit_of_everything.proto#L286) example). In this case, the FieldMask is updated from the request body and set in the gRPC request message.
- The FieldMask is exposed to the REST request (as in the [PatchWithFieldMaskInBody](https://github.com/grpc-ecosystem/grpc-gateway/blob/master/examples/proto/examplepb/a_bit_of_everything.proto#L295)  example). For this case, a check is made as to whether the FieldMask is nil/empty prior to populating with the request body. 

If it's not nil, then it converts the FieldMask paths from the REST (snake_case) to gRPC (PascalCase) format. Otherwise, it acts like the previous case.

## Example Usage
1. Create PATCH request. 

    The PATCH request needs to include the message and the update mask.
```golang 
// UpdateV2Request request for update includes the message and the update mask
message UpdateV2Request {
	ABitOfEverything abe = 1;
	google.protobuf.FieldMask update_mask = 2;
}
```
2. Define your service in gRPC

If you want to use PATCH with fieldmask hidden from REST request only include the request message in the body.

```golang
rpc UpdateV2(UpdateV2Request) returns (google.protobuf.Empty) {
	option (google.api.http) = {
		put: "/v2/example/a_bit_of_everything/{abe.uuid}"
		body: "abe"
		additional_bindings {
			patch: "/v2/example/a_bit_of_everything/{abe.uuid}"
			body: "abe"
		}
	};
}
```

If you want to use PATCH wtih fieldmask exposed to the REST request then include the entire request message.

```golang
rpc PatchWithFieldMaskInBody(UpdateV2Request) returns (google.protobuf.Empty) {
	option (google.api.http) = {
		patch: "/v2a/example/a_bit_of_everything/{abe.uuid}"
		body: "*"
    };
}
```

3. Generate gRPC and reverse-proxy stubs and implement your service.

## Curl examples

In the example below we will partially update our ABitOfEverything resource by passing only the field we want to change. Since we are using the endpoint with field mask hidden we only need to pass the field we want to change ("string_value") and it will keep everything else in our resource the same.
```
curl --data '{"string_value": "strprefix/foo"}' -X PATCH http://address:port/v2/example/a_bit_of_everything/1
```

If we know what fields we want to update then we can use PATCH with field mask approach. For this we need to pass the resource and the update_mask. Below only the "single_nested" will get updated because that is what we specify in the field_mask.
```
curl --data '{"abe":{"single_nested":{"amount":457},"string_value":"some value that won't get updated because not in the field mask"},"update_mask":{"paths":["single_nested"]}}' -X PATCH http://address:port/v2a/example/a_bit_of_everything/1
```