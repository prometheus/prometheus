# Change Log

## [v1.9.5](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.9.5) (2019-07-22)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.9.4...v1.9.5)

**Fixed bugs:**

- Non-standard use of 412 HTTP Status Code [\#972](https://github.com/grpc-ecosystem/grpc-gateway/issues/972)

**Closed issues:**

- why response use enum's name [\#970](https://github.com/grpc-ecosystem/grpc-gateway/issues/970)

**Merged pull requests:**

- Fix HTTP Status Code returned for a `Failed Precondition` error [\#974](https://github.com/grpc-ecosystem/grpc-gateway/pull/974) ([cjcormack](https://github.com/cjcormack))
- Examples fix: Support preflight of auth libraries in js [\#973](https://github.com/grpc-ecosystem/grpc-gateway/pull/973) ([GhiaC](https://github.com/GhiaC))

## [v1.9.4](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.9.4) (2019-07-09)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.9.3...v1.9.4)

**Closed issues:**

- Read the Http Post Body  [\#921](https://github.com/grpc-ecosystem/grpc-gateway/issues/921)
- Swagger document generation, required field is invalid [\#665](https://github.com/grpc-ecosystem/grpc-gateway/issues/665)

**Merged pull requests:**

- Generate changelog for 1.9.4 [\#969](https://github.com/grpc-ecosystem/grpc-gateway/pull/969) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Fix query.go to avoid invalid protobuf assumptions [\#967](https://github.com/grpc-ecosystem/grpc-gateway/pull/967) ([dsnet](https://github.com/dsnet))
- doc\(readme\): fix typo [\#965](https://github.com/grpc-ecosystem/grpc-gateway/pull/965) ([franxois](https://github.com/franxois))
- Added comments to base\_path to explain behavior [\#919](https://github.com/grpc-ecosystem/grpc-gateway/pull/919) ([nu11ptr](https://github.com/nu11ptr))

## [v1.9.3](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.9.3) (2019-06-28)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.9.2...v1.9.3)

**Fixed bugs:**

- EOF when calling Send for client streams [\#961](https://github.com/grpc-ecosystem/grpc-gateway/issues/961)

**Closed issues:**

- Please make a new release! [\#963](https://github.com/grpc-ecosystem/grpc-gateway/issues/963)
- application/x-www-form-urlencoded support. [\#960](https://github.com/grpc-ecosystem/grpc-gateway/issues/960)
- Bazel files are out of date [\#955](https://github.com/grpc-ecosystem/grpc-gateway/issues/955)
- Configurable AllowUnknownFields in jsonpb? [\#448](https://github.com/grpc-ecosystem/grpc-gateway/issues/448)

**Merged pull requests:**

- Generate changelog for 1.9.3 [\#964](https://github.com/grpc-ecosystem/grpc-gateway/pull/964) ([johanbrandhorst](https://github.com/johanbrandhorst))
- EOF on send [\#962](https://github.com/grpc-ecosystem/grpc-gateway/pull/962) ([gustavocovas](https://github.com/gustavocovas))
- Add new option for the decoder - to disallow unknown fields [\#959](https://github.com/grpc-ecosystem/grpc-gateway/pull/959) ([vsaveliev](https://github.com/vsaveliev))
- Update to rules\_go and buildtools [\#956](https://github.com/grpc-ecosystem/grpc-gateway/pull/956) ([drigz](https://github.com/drigz))
- docs/customizingyourgateway: add ?pretty example [\#954](https://github.com/grpc-ecosystem/grpc-gateway/pull/954) ([srenatus](https://github.com/srenatus))
- protoc\_gen\_swagger: Add attr for allow\_merge [\#944](https://github.com/grpc-ecosystem/grpc-gateway/pull/944) ([prestonvanloon](https://github.com/prestonvanloon))

## [v1.9.2](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.9.2) (2019-06-17)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.9.1...v1.9.2)

**Fixed bugs:**

- 404s using colons in the middle of the last path segment [\#224](https://github.com/grpc-ecosystem/grpc-gateway/issues/224)

**Merged pull requests:**

- Generate changelog for 1.9.2 [\#953](https://github.com/grpc-ecosystem/grpc-gateway/pull/953) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Improve README file [\#950](https://github.com/grpc-ecosystem/grpc-gateway/pull/950) ([charleswhchan](https://github.com/charleswhchan))
- Support colon in final path segment, last match wins behavior \(behind flags\) [\#949](https://github.com/grpc-ecosystem/grpc-gateway/pull/949) ([jfhamlin](https://github.com/jfhamlin))

## [v1.9.1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.9.1) (2019-06-13)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.9.0...v1.9.1)

**Closed issues:**

- grpc: received message larger than max [\#943](https://github.com/grpc-ecosystem/grpc-gateway/issues/943)
- json 1.1 api support for grpc-ecosystem to use queryparams with filter [\#938](https://github.com/grpc-ecosystem/grpc-gateway/issues/938)
- i import a new gateway.Endpoint without recompile [\#937](https://github.com/grpc-ecosystem/grpc-gateway/issues/937)
- all SubConns are in TransientFailure [\#936](https://github.com/grpc-ecosystem/grpc-gateway/issues/936)
- Merging swagger specs fails to use rpc comments \(again\) [\#923](https://github.com/grpc-ecosystem/grpc-gateway/issues/923)

**Merged pull requests:**

- Generate changelog for 1.9.1 [\#946](https://github.com/grpc-ecosystem/grpc-gateway/pull/946) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Revert "protoc-gen-swagger: check typeIndex when typeName is Method" [\#945](https://github.com/grpc-ecosystem/grpc-gateway/pull/945) ([johanbrandhorst](https://github.com/johanbrandhorst))
- fix query params not populate if method is post [\#939](https://github.com/grpc-ecosystem/grpc-gateway/pull/939) ([mingqing](https://github.com/mingqing))
- Fix make test on MacOS [\#935](https://github.com/grpc-ecosystem/grpc-gateway/pull/935) ([emilaasa](https://github.com/emilaasa))

## [v1.9.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.9.0) (2019-05-14)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.8.6...v1.9.0)

**Closed issues:**

- Errors in response streams do not go through the registered error handler [\#584](https://github.com/grpc-ecosystem/grpc-gateway/issues/584)

**Merged pull requests:**

- Generate changelog for 1.9.0 [\#933](https://github.com/grpc-ecosystem/grpc-gateway/pull/933) ([johanbrandhorst](https://github.com/johanbrandhorst))
- use error value for bad URI so custom error handler could treat it special [\#932](https://github.com/grpc-ecosystem/grpc-gateway/pull/932) ([jhump](https://github.com/jhump))
- newline between JSON messages [\#931](https://github.com/grpc-ecosystem/grpc-gateway/pull/931) ([jhump](https://github.com/jhump))
- ability to customize stream errors [\#930](https://github.com/grpc-ecosystem/grpc-gateway/pull/930) ([jhump](https://github.com/jhump))

## [v1.8.6](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.8.6) (2019-05-07)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.8.5...v1.8.6)

**Fixed bugs:**

- can't specify an empty path? [\#414](https://github.com/grpc-ecosystem/grpc-gateway/issues/414)

**Closed issues:**

- JSON stream response not available [\#926](https://github.com/grpc-ecosystem/grpc-gateway/issues/926)
- why google/api/http.proto annotations.proto Field Numbers is 72295728 ? [\#925](https://github.com/grpc-ecosystem/grpc-gateway/issues/925)
- Documentation: 'base\_path' Swagger attribute confuses users [\#918](https://github.com/grpc-ecosystem/grpc-gateway/issues/918)
- go get: error loading module requirements go 1.11 [\#915](https://github.com/grpc-ecosystem/grpc-gateway/issues/915)
- gateway generation issue on windows [\#911](https://github.com/grpc-ecosystem/grpc-gateway/issues/911)

**Merged pull requests:**

- Generate correct changelog version [\#929](https://github.com/grpc-ecosystem/grpc-gateway/pull/929) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Generate changelog for 1.8.6 [\#928](https://github.com/grpc-ecosystem/grpc-gateway/pull/928) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Use proto gen swagger with protos from external repository [\#924](https://github.com/grpc-ecosystem/grpc-gateway/pull/924) ([elenadeneva92](https://github.com/elenadeneva92))
- Delete redundant load statement [\#922](https://github.com/grpc-ecosystem/grpc-gateway/pull/922) ([pcj](https://github.com/pcj))
- Make gazelle diffs fail the build [\#916](https://github.com/grpc-ecosystem/grpc-gateway/pull/916) ([achew22](https://github.com/achew22))
- Fixed empty path bug [\#913](https://github.com/grpc-ecosystem/grpc-gateway/pull/913) ([brycematheson1234](https://github.com/brycematheson1234))

## [v1.8.5](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.8.5) (2019-03-15)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.8.4...v1.8.5)

**Closed issues:**

- Swagger get query param documentation shows repeated fields incorrectly [\#756](https://github.com/grpc-ecosystem/grpc-gateway/issues/756)

**Merged pull requests:**

- Generate changelog for 1.8.5 [\#910](https://github.com/grpc-ecosystem/grpc-gateway/pull/910) ([johanbrandhorst](https://github.com/johanbrandhorst))
- CollectionFormat multi for query params of repeated fields 2 [\#909](https://github.com/grpc-ecosystem/grpc-gateway/pull/909) ([bmperrea](https://github.com/bmperrea))

## [v1.8.4](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.8.4) (2019-03-13)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.8.3...v1.8.4)

**Closed issues:**

- Invalid swagger generated for bodies with repeated fields [\#906](https://github.com/grpc-ecosystem/grpc-gateway/issues/906)

**Merged pull requests:**

- Generate changelog for 1.8.4 [\#908](https://github.com/grpc-ecosystem/grpc-gateway/pull/908) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Revert "Use collectionFormat multi for query params of repeated fields \(\#902\)" [\#907](https://github.com/grpc-ecosystem/grpc-gateway/pull/907) ([johanbrandhorst](https://github.com/johanbrandhorst))
- New proposal: support for the google.api.HttpBody [\#904](https://github.com/grpc-ecosystem/grpc-gateway/pull/904) ([wimspaargaren](https://github.com/wimspaargaren))

## [v1.8.3](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.8.3) (2019-03-11)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.8.2...v1.8.3)

**Implemented enhancements:**

- Feature request from openapi 3: Allow apiKey in cookie [\#900](https://github.com/grpc-ecosystem/grpc-gateway/issues/900)

**Fixed bugs:**

- Error while defining enum comments [\#897](https://github.com/grpc-ecosystem/grpc-gateway/issues/897)

**Closed issues:**

- Its impossible to send response with non 200 status code [\#901](https://github.com/grpc-ecosystem/grpc-gateway/issues/901)

**Merged pull requests:**

- Regenerate changelog for 1.8.3 [\#903](https://github.com/grpc-ecosystem/grpc-gateway/pull/903) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Use collectionFormat multi for query params of repeated fields [\#902](https://github.com/grpc-ecosystem/grpc-gateway/pull/902) ([bmperrea](https://github.com/bmperrea))

## [v1.8.2](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.8.2) (2019-03-07)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.8.1...v1.8.2)

**Implemented enhancements:**

- Update the build environment Dockerfile to Go 1.12 [\#885](https://github.com/grpc-ecosystem/grpc-gateway/issues/885)

**Fixed bugs:**

- Change in behavior of streaming request body \(1.4.1 vs 1.8.1\) [\#894](https://github.com/grpc-ecosystem/grpc-gateway/issues/894)
- Cannot download 1.8.0 with modules [\#886](https://github.com/grpc-ecosystem/grpc-gateway/issues/886)

**Closed issues:**

- Description and title ignored when field is not a scaler value type [\#892](https://github.com/grpc-ecosystem/grpc-gateway/issues/892)

**Merged pull requests:**

- Regenerate changelog for 1.8.2 [\#899](https://github.com/grpc-ecosystem/grpc-gateway/pull/899) ([johanbrandhorst](https://github.com/johanbrandhorst))
- 897 fixing problem while generating swagger documentation for enum messages  [\#898](https://github.com/grpc-ecosystem/grpc-gateway/pull/898) ([fahernandez](https://github.com/fahernandez))
- bugfix: disable IOReaderFactory for streaming requests [\#896](https://github.com/grpc-ecosystem/grpc-gateway/pull/896) ([happyalu](https://github.com/happyalu))
- bazel: Use new ProtoInfo provider [\#893](https://github.com/grpc-ecosystem/grpc-gateway/pull/893) ([drigz](https://github.com/drigz))
- README: Add some nicer looking badges [\#890](https://github.com/grpc-ecosystem/grpc-gateway/pull/890) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Upgrade generator and runtime versions [\#889](https://github.com/grpc-ecosystem/grpc-gateway/pull/889) ([johanbrandhorst](https://github.com/johanbrandhorst))

## [v1.8.1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.8.1) (2019-03-02)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.8.1-pre1...v1.8.1)

**Merged pull requests:**

- Generate changelog for v1.8.1 [\#887](https://github.com/grpc-ecosystem/grpc-gateway/pull/887) ([johanbrandhorst](https://github.com/johanbrandhorst))

## [v1.8.1-pre1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.8.1-pre1) (2019-03-01)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.8.0...v1.8.1-pre1)

**Merged pull requests:**

- CI: fix release builds [\#884](https://github.com/grpc-ecosystem/grpc-gateway/pull/884) ([johanbrandhorst](https://github.com/johanbrandhorst))

## [v1.8.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.8.0) (2019-03-01)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.7.0...v1.8.0)

**Implemented enhancements:**

- Support swagger annotations for default and required fields [\#851](https://github.com/grpc-ecosystem/grpc-gateway/issues/851)
- Support go modules [\#755](https://github.com/grpc-ecosystem/grpc-gateway/issues/755)

**Fixed bugs:**

- inconsistent identifier capitalization between protoc-gen-go and protoc-gen-grpc-gateway [\#683](https://github.com/grpc-ecosystem/grpc-gateway/issues/683)

**Closed issues:**

- Bazel incompatible changes [\#873](https://github.com/grpc-ecosystem/grpc-gateway/issues/873)
- Swagger has not existed for four years [\#872](https://github.com/grpc-ecosystem/grpc-gateway/issues/872)
- Improve README with AWS API Gateway findings [\#868](https://github.com/grpc-ecosystem/grpc-gateway/issues/868)
- swagger error [\#867](https://github.com/grpc-ecosystem/grpc-gateway/issues/867)
- A question about generating file protoc-gen-grpc-gateway/gengateway/template.go [\#864](https://github.com/grpc-ecosystem/grpc-gateway/issues/864)
- Repeated field documentation is overwritten by fields comments [\#863](https://github.com/grpc-ecosystem/grpc-gateway/issues/863)
- Using dep to depend on specific revision of golang/protobuf is causing transative dependency problems for users [\#829](https://github.com/grpc-ecosystem/grpc-gateway/issues/829)
- Mac OS X - Note about your tutorial [\#787](https://github.com/grpc-ecosystem/grpc-gateway/issues/787)
- Returning 302 redirect as response [\#607](https://github.com/grpc-ecosystem/grpc-gateway/issues/607)

**Merged pull requests:**

- Generate changelog for 1.8.0 [\#883](https://github.com/grpc-ecosystem/grpc-gateway/pull/883) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Read only support [\#882](https://github.com/grpc-ecosystem/grpc-gateway/pull/882) ([hypnoce](https://github.com/hypnoce))
- Add fqn\_for\_swagger\_name option [\#881](https://github.com/grpc-ecosystem/grpc-gateway/pull/881) ([hypnoce](https://github.com/hypnoce))
- go.mod: update grpc from v1.16.0 to v1.17.0 [\#880](https://github.com/grpc-ecosystem/grpc-gateway/pull/880) ([klim0v](https://github.com/klim0v))
- Fix parameter names when using JSON names. [\#879](https://github.com/grpc-ecosystem/grpc-gateway/pull/879) ([brocaar](https://github.com/brocaar))
- protoc-gen-swagger: return error when encoding swagger file [\#878](https://github.com/grpc-ecosystem/grpc-gateway/pull/878) ([elliots](https://github.com/elliots))
- protoc-gen-grpc-gateway: use context package from stdlib [\#876](https://github.com/grpc-ecosystem/grpc-gateway/pull/876) ([simonpasquier](https://github.com/simonpasquier))
- Run buildifer on WORKSPACE [\#875](https://github.com/grpc-ecosystem/grpc-gateway/pull/875) ([achew22](https://github.com/achew22))
- Upgrade to rules\_go 0.17.0 [\#874](https://github.com/grpc-ecosystem/grpc-gateway/pull/874) ([achew22](https://github.com/achew22))
- Switch to go modules [\#870](https://github.com/grpc-ecosystem/grpc-gateway/pull/870) ([johanbrandhorst](https://github.com/johanbrandhorst))
- 868 improving README with AWS API gateway findings [\#869](https://github.com/grpc-ecosystem/grpc-gateway/pull/869) ([fahernandez](https://github.com/fahernandez))
- Updated Service, Method, Message Identifiers to be CamelCased [\#866](https://github.com/grpc-ecosystem/grpc-gateway/pull/866) ([waveywaves](https://github.com/waveywaves))
- 863 adding swagger annotation support for enum and nested objects [\#865](https://github.com/grpc-ecosystem/grpc-gateway/pull/865) ([fahernandez](https://github.com/fahernandez))
- Update CI badge link in documentation [\#862](https://github.com/grpc-ecosystem/grpc-gateway/pull/862) ([johanbrandhorst](https://github.com/johanbrandhorst))
- protoc-gen-swagger: add the package name to the tags field of each endpoint if the package name exists in the proto file [\#860](https://github.com/grpc-ecosystem/grpc-gateway/pull/860) ([zwcn](https://github.com/zwcn))

## [v1.7.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.7.0) (2019-01-23)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.6.4...v1.7.0)

**Closed issues:**

- Error to build project with go module [\#846](https://github.com/grpc-ecosystem/grpc-gateway/issues/846)
- Result of gateway's Stream response is wrapped with "result" [\#579](https://github.com/grpc-ecosystem/grpc-gateway/issues/579)

**Merged pull requests:**

- Generate changelog for 1.7.0 [\#858](https://github.com/grpc-ecosystem/grpc-gateway/pull/858) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Use github.com/golang/protobuf/descriptor ForMessage and fix CI from not rebasing [\#857](https://github.com/grpc-ecosystem/grpc-gateway/pull/857) ([mechinn](https://github.com/mechinn))
- marshal\_jsonpb: add check for slice sub types implementing proto.Message [\#856](https://github.com/grpc-ecosystem/grpc-gateway/pull/856) ([abice](https://github.com/abice))
- Added WithDisablePathLengthFallback option \(to fix issue \#447\) [\#855](https://github.com/grpc-ecosystem/grpc-gateway/pull/855) ([UladzimirTrehubenka](https://github.com/UladzimirTrehubenka))
- marshal\_jsonpb: Added nil slice default value [\#854](https://github.com/grpc-ecosystem/grpc-gateway/pull/854) ([abice](https://github.com/abice))
- Add flag 'allow\_repeated\_fields\_in\_body' to protoc-gen-swagger [\#853](https://github.com/grpc-ecosystem/grpc-gateway/pull/853) ([abice](https://github.com/abice))
- Adding support for default and required swagger annotation fields. [\#852](https://github.com/grpc-ecosystem/grpc-gateway/pull/852) ([fahernandez](https://github.com/fahernandez))
- make generated swagger json match gateway behavior for server streams [\#850](https://github.com/grpc-ecosystem/grpc-gateway/pull/850) ([mechinn](https://github.com/mechinn))
- test: "fill attributes of swagger schema if provided for messages" [\#849](https://github.com/grpc-ecosystem/grpc-gateway/pull/849) ([srenatus](https://github.com/srenatus))
- Fix the generated URL in the changelog [\#845](https://github.com/grpc-ecosystem/grpc-gateway/pull/845) ([johanbrandhorst](https://github.com/johanbrandhorst))

## [v1.6.4](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.6.4) (2019-01-08)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.6.3...v1.6.4)

**Closed issues:**

- feature request: opt-out fieldmask behaviour in patch [\#839](https://github.com/grpc-ecosystem/grpc-gateway/issues/839)
- gRPC streaming keepAlive doesn't work with docker swarm [\#838](https://github.com/grpc-ecosystem/grpc-gateway/issues/838)

**Merged pull requests:**

- Generate changelog for 1.6.4 [\#843](https://github.com/grpc-ecosystem/grpc-gateway/pull/843) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Update bazel dependencies [\#841](https://github.com/grpc-ecosystem/grpc-gateway/pull/841) ([achew22](https://github.com/achew22))
- gengateway: allow opting out patch feature [\#840](https://github.com/grpc-ecosystem/grpc-gateway/pull/840) ([glerchundi](https://github.com/glerchundi))
- Fix the url of gRPC timeouts on README.md [\#836](https://github.com/grpc-ecosystem/grpc-gateway/pull/836) ([royeo](https://github.com/royeo))

## [v1.6.3](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.6.3) (2018-12-21)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.6.2...v1.6.3)

**Closed issues:**

- Issue with google.protobuf.Empty representation in swagger file [\#831](https://github.com/grpc-ecosystem/grpc-gateway/issues/831)

**Merged pull requests:**

- Regenerate changelog for 1.6.3 [\#835](https://github.com/grpc-ecosystem/grpc-gateway/pull/835) ([johanbrandhorst](https://github.com/johanbrandhorst))
- protoc-gen-swagger: check typeIndex when typeName is Method [\#833](https://github.com/grpc-ecosystem/grpc-gateway/pull/833) ([hexfusion](https://github.com/hexfusion))
- Replace complicated circle CI release with goreleaser. [\#828](https://github.com/grpc-ecosystem/grpc-gateway/pull/828) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Stop the publishing recursion [\#827](https://github.com/grpc-ecosystem/grpc-gateway/pull/827) ([achew22](https://github.com/achew22))

## [v1.6.2](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.6.2) (2018-12-07)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.6.0...v1.6.2)

## [v1.6.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.6.0) (2018-12-07)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.6.1...v1.6.0)

## [v1.6.1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.6.1) (2018-12-07)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.5.1...v1.6.1)

**Implemented enhancements:**

- Add 'License' message to the annotation proto. [\#644](https://github.com/grpc-ecosystem/grpc-gateway/pull/644) ([ensonic](https://github.com/ensonic))

**Fixed bugs:**

- Cannot return HTTP header using "Grpc-Metadata-" prefix [\#782](https://github.com/grpc-ecosystem/grpc-gateway/issues/782)
- protoc-gen-swagger  throws the error: Any JSON doesn't have '@type' [\#771](https://github.com/grpc-ecosystem/grpc-gateway/issues/771)
- proto-gen-swagger: provide default description for HTTP 200 responses [\#766](https://github.com/grpc-ecosystem/grpc-gateway/issues/766)

**Closed issues:**

- Please release the repo, IOReaderFactory is not available on the latest release! [\#823](https://github.com/grpc-ecosystem/grpc-gateway/issues/823)
- Bazel CI breaks frequently [\#817](https://github.com/grpc-ecosystem/grpc-gateway/issues/817)
- Unable to add protobuf wrappers in url template option [\#808](https://github.com/grpc-ecosystem/grpc-gateway/issues/808)
- Class 'GPBMetadata\ProtocGenSwagger\Options\Annotations' not found [\#794](https://github.com/grpc-ecosystem/grpc-gateway/issues/794)
- REST gateway over RPCS? [\#789](https://github.com/grpc-ecosystem/grpc-gateway/issues/789)
- Why the rctx is substituted by a new empty context? [\#788](https://github.com/grpc-ecosystem/grpc-gateway/issues/788)
- grpc gateway intercepter [\#785](https://github.com/grpc-ecosystem/grpc-gateway/issues/785)
- "error" and "message" fields in error response [\#768](https://github.com/grpc-ecosystem/grpc-gateway/issues/768)
- Go1.11: http.CloseNotifier is deprecated [\#736](https://github.com/grpc-ecosystem/grpc-gateway/issues/736)
- Access to raw post body [\#652](https://github.com/grpc-ecosystem/grpc-gateway/issues/652)

**Merged pull requests:**

- Write version to intermediate file for release publish [\#826](https://github.com/grpc-ecosystem/grpc-gateway/pull/826) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Check out code before calling ghr [\#825](https://github.com/grpc-ecosystem/grpc-gateway/pull/825) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Generate changelog for 1.6.0 [\#824](https://github.com/grpc-ecosystem/grpc-gateway/pull/824) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Add filegroup for options proto files [\#821](https://github.com/grpc-ecosystem/grpc-gateway/pull/821) ([kellycampbell](https://github.com/kellycampbell))
- Added support for more WKT [\#816](https://github.com/grpc-ecosystem/grpc-gateway/pull/816) ([mayankcpdixit](https://github.com/mayankcpdixit))
- Fix protobuf repository's owner name on README.md [\#814](https://github.com/grpc-ecosystem/grpc-gateway/pull/814) ([micnncim](https://github.com/micnncim))
- Revert "Adding support for more well known types in descriptor" [\#813](https://github.com/grpc-ecosystem/grpc-gateway/pull/813) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Feature/patch2 rebased [\#812](https://github.com/grpc-ecosystem/grpc-gateway/pull/812) ([razamiDev](https://github.com/razamiDev))
- Correct wellKnownTypeConv function references [\#811](https://github.com/grpc-ecosystem/grpc-gateway/pull/811) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Adding support for more well known types in descriptor [\#809](https://github.com/grpc-ecosystem/grpc-gateway/pull/809) ([mayankcpdixit](https://github.com/mayankcpdixit))
- Make Bazel CI failures clearer [\#807](https://github.com/grpc-ecosystem/grpc-gateway/pull/807) ([drigz](https://github.com/drigz))
- fix bug: the resource name of custom method doesn't be retained [\#805](https://github.com/grpc-ecosystem/grpc-gateway/pull/805) ([ch3rub1m](https://github.com/ch3rub1m))
- Update rules\_go and gazelle [\#802](https://github.com/grpc-ecosystem/grpc-gateway/pull/802) ([drigz](https://github.com/drigz))
- Properly omit wrappers and google.protobuf.empty from swagger definitions [\#801](https://github.com/grpc-ecosystem/grpc-gateway/pull/801) ([birdayz](https://github.com/birdayz))
- protoc-gen-swagger: honor example field of message option [\#799](https://github.com/grpc-ecosystem/grpc-gateway/pull/799) ([birdayz](https://github.com/birdayz))
- Use newer Bazel repo rules [\#798](https://github.com/grpc-ecosystem/grpc-gateway/pull/798) ([drigz](https://github.com/drigz))
- Run buildifer on Bazel files [\#797](https://github.com/grpc-ecosystem/grpc-gateway/pull/797) ([drigz](https://github.com/drigz))
- protoc-gen-swagger: Fix formatting of license field definition. [\#796](https://github.com/grpc-ecosystem/grpc-gateway/pull/796) ([ivucica](https://github.com/ivucica))
- Remove http.CloseNotifier code from Go \>= 1.7 builds [\#795](https://github.com/grpc-ecosystem/grpc-gateway/pull/795) ([SpikesDivZero](https://github.com/SpikesDivZero))
- ci: add job for building binaries for releases [\#793](https://github.com/grpc-ecosystem/grpc-gateway/pull/793) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Add documentation to the rest of methods on the examples [\#791](https://github.com/grpc-ecosystem/grpc-gateway/pull/791) ([rvegas](https://github.com/rvegas))
- fix \#782 Cannot return HTTP header using "Grpc-Metadata-" prefix [\#784](https://github.com/grpc-ecosystem/grpc-gateway/pull/784) ([joelclouddistrict](https://github.com/joelclouddistrict))
- Fix CircleCI configuration [\#777](https://github.com/grpc-ecosystem/grpc-gateway/pull/777) ([johanbrandhorst](https://github.com/johanbrandhorst))
- tests: s/iotuil/ioutil/ typo [\#775](https://github.com/grpc-ecosystem/grpc-gateway/pull/775) ([srenatus](https://github.com/srenatus))
- Replace travis with CircleCI for easier testing [\#772](https://github.com/grpc-ecosystem/grpc-gateway/pull/772) ([johanbrandhorst](https://github.com/johanbrandhorst))

## [v1.5.1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.5.1) (2018-10-02)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.5.0...v1.5.1)

**Implemented enhancements:**

- protobuf well known types aren't represented in swagger output correctly [\#160](https://github.com/grpc-ecosystem/grpc-gateway/issues/160)

**Fixed bugs:**

- URLs using verb no longer work after upgrading to v1.5.0 [\#760](https://github.com/grpc-ecosystem/grpc-gateway/issues/760)
- protoc-gen-swagger doesn't generate any request objects for GET/DELETE [\#747](https://github.com/grpc-ecosystem/grpc-gateway/issues/747)

**Closed issues:**

- how to get proper fields name for method [\#745](https://github.com/grpc-ecosystem/grpc-gateway/issues/745)
- Make a new release [\#733](https://github.com/grpc-ecosystem/grpc-gateway/issues/733)
- how to provide interface type inside proto for grpc-gateway [\#723](https://github.com/grpc-ecosystem/grpc-gateway/issues/723)
- Is there any way we can remove fields from the response json in grpc-gateway? [\#710](https://github.com/grpc-ecosystem/grpc-gateway/issues/710)
- How to write tests for the gateway? [\#699](https://github.com/grpc-ecosystem/grpc-gateway/issues/699)
- protoc-gen-swagger: No comments for path parameters [\#694](https://github.com/grpc-ecosystem/grpc-gateway/issues/694)
- Can you differentiate between an empty map vs field not provided? [\#552](https://github.com/grpc-ecosystem/grpc-gateway/issues/552)
- import\_path option not working as intended [\#536](https://github.com/grpc-ecosystem/grpc-gateway/issues/536)

**Merged pull requests:**

- Add default value for swagger 200 response [\#767](https://github.com/grpc-ecosystem/grpc-gateway/pull/767) ([johnchildren](https://github.com/johnchildren))
- Generate changelog for release v1.5.1 [\#764](https://github.com/grpc-ecosystem/grpc-gateway/pull/764) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Revert \#708 since it breaks backwards compatibility [\#761](https://github.com/grpc-ecosystem/grpc-gateway/pull/761) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Update README.md [\#757](https://github.com/grpc-ecosystem/grpc-gateway/pull/757) ([wora](https://github.com/wora))
- Added camelCase Example [\#751](https://github.com/grpc-ecosystem/grpc-gateway/pull/751) ([srikrsna](https://github.com/srikrsna))
- Add more guidance to issue template [\#750](https://github.com/grpc-ecosystem/grpc-gateway/pull/750) ([johanbrandhorst](https://github.com/johanbrandhorst))

## [v1.5.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.5.0) (2018-09-09)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.4.1...v1.5.0)

**Fixed bugs:**

- forwarding binary metadata is broken [\#218](https://github.com/grpc-ecosystem/grpc-gateway/issues/218)

**Closed issues:**

- something wrong with service [\#748](https://github.com/grpc-ecosystem/grpc-gateway/issues/748)
- Support for repeated path parameters [\#741](https://github.com/grpc-ecosystem/grpc-gateway/issues/741)
- Uint64 is represented as type:"string" in the swagger docs. [\#735](https://github.com/grpc-ecosystem/grpc-gateway/issues/735)
- autoregister all provided services [\#732](https://github.com/grpc-ecosystem/grpc-gateway/issues/732)
- `go get -u github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway` fails on clean environment [\#731](https://github.com/grpc-ecosystem/grpc-gateway/issues/731)
- format tool [\#729](https://github.com/grpc-ecosystem/grpc-gateway/issues/729)
- how to do tls auth in grpc+grpc-gateway [\#727](https://github.com/grpc-ecosystem/grpc-gateway/issues/727)
- Let service choose it's own marshaller [\#725](https://github.com/grpc-ecosystem/grpc-gateway/issues/725)
- why gateway proxy can not distribute the http request to local server?  prompt 404 [\#722](https://github.com/grpc-ecosystem/grpc-gateway/issues/722)
- enc.SetIndent undefined \(type \*json.Encoder has no field or method SetIndent\) [\#717](https://github.com/grpc-ecosystem/grpc-gateway/issues/717)
- Travis CI fails on master branch [\#714](https://github.com/grpc-ecosystem/grpc-gateway/issues/714)
- google/protobuf/descriptor.proto: File not found. ? [\#713](https://github.com/grpc-ecosystem/grpc-gateway/issues/713)
- APIs with grpc-gateway \(S3,WebDav\) [\#709](https://github.com/grpc-ecosystem/grpc-gateway/issues/709)
- FR: Promote a field in the returned JSON message to a top-level returned value [\#707](https://github.com/grpc-ecosystem/grpc-gateway/issues/707)
- Does grpc-gateway support the HTTP 2.0 protocol? [\#703](https://github.com/grpc-ecosystem/grpc-gateway/issues/703)
- The swagger plugin couldn’t distinguish two rpcs if we use the resource name design style. [\#702](https://github.com/grpc-ecosystem/grpc-gateway/issues/702)
- Handling of optional parameters [\#697](https://github.com/grpc-ecosystem/grpc-gateway/issues/697)
- Vendor dependencies [\#689](https://github.com/grpc-ecosystem/grpc-gateway/issues/689)
- Output swagger seems incorrect [\#688](https://github.com/grpc-ecosystem/grpc-gateway/issues/688)
- how to use this in java? [\#685](https://github.com/grpc-ecosystem/grpc-gateway/issues/685)
- r [\#684](https://github.com/grpc-ecosystem/grpc-gateway/issues/684)
- url query parameters should support semicolon in value field [\#680](https://github.com/grpc-ecosystem/grpc-gateway/issues/680)
- how to install swagger-codegen@2.2.2? [\#670](https://github.com/grpc-ecosystem/grpc-gateway/issues/670)
- Merging swagger specs fails to use rpc comments [\#664](https://github.com/grpc-ecosystem/grpc-gateway/issues/664)
- Impossible to use gogo/protobuf registered types in gRPC Status errors [\#576](https://github.com/grpc-ecosystem/grpc-gateway/issues/576)
- Path parameters can't have URL encoded values [\#566](https://github.com/grpc-ecosystem/grpc-gateway/issues/566)
- docs: show example of tracing over http-\>grpc boundary [\#348](https://github.com/grpc-ecosystem/grpc-gateway/issues/348)
- Response codes and descriptions in Swagger docs [\#304](https://github.com/grpc-ecosystem/grpc-gateway/issues/304)

**Merged pull requests:**

- Generate release notes for v1.5.0 [\#749](https://github.com/grpc-ecosystem/grpc-gateway/pull/749) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Add missing modules to browser example [\#743](https://github.com/grpc-ecosystem/grpc-gateway/pull/743) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Added support for path param repeated fields [\#742](https://github.com/grpc-ecosystem/grpc-gateway/pull/742) ([maros7](https://github.com/maros7))
- Add support for enum path parameters [\#738](https://github.com/grpc-ecosystem/grpc-gateway/pull/738) ([maros7](https://github.com/maros7))
- Add support to forward grpc binary metadata [\#737](https://github.com/grpc-ecosystem/grpc-gateway/pull/737) ([timonwong](https://github.com/timonwong))
- Lock versions to tags where possible [\#724](https://github.com/grpc-ecosystem/grpc-gateway/pull/724) ([johanbrandhorst](https://github.com/johanbrandhorst))
- install-protoc was checking version from wrong executable path [\#721](https://github.com/grpc-ecosystem/grpc-gateway/pull/721) ([temoto](https://github.com/temoto))
- Fix naming convention of JSON Schema didn't matched with the spec [\#719](https://github.com/grpc-ecosystem/grpc-gateway/pull/719) ([co3k](https://github.com/co3k))
- Add message field to the error message emitted by grpc-gateway [\#718](https://github.com/grpc-ecosystem/grpc-gateway/pull/718) ([ffredsh](https://github.com/ffredsh))
- Fix up examples [\#715](https://github.com/grpc-ecosystem/grpc-gateway/pull/715) ([achew22](https://github.com/achew22))
- Support HttpRule with field response [\#712](https://github.com/grpc-ecosystem/grpc-gateway/pull/712) ([doroginin](https://github.com/doroginin))
- Make support paths option [\#711](https://github.com/grpc-ecosystem/grpc-gateway/pull/711) ([izumin5210](https://github.com/izumin5210))
- Add test case and proposed fix for path component with trailing colon \(and string\) [\#708](https://github.com/grpc-ecosystem/grpc-gateway/pull/708) ([jfhamlin](https://github.com/jfhamlin))
- add OpenTracing support to docs [\#705](https://github.com/grpc-ecosystem/grpc-gateway/pull/705) ([theRealWardo](https://github.com/theRealWardo))
- add support for resource name in swagger plugin \(\#702\) [\#704](https://github.com/grpc-ecosystem/grpc-gateway/pull/704) ([ch3rub1m](https://github.com/ch3rub1m))
- Add explicit dependency versions [\#696](https://github.com/grpc-ecosystem/grpc-gateway/pull/696) ([johanbrandhorst](https://github.com/johanbrandhorst))
- protoc-gen-swagger: support all well-known wrapper types [\#695](https://github.com/grpc-ecosystem/grpc-gateway/pull/695) ([jriecken](https://github.com/jriecken))
- runtime: add support for time types in query parameters [\#693](https://github.com/grpc-ecosystem/grpc-gateway/pull/693) ([johanbrandhorst](https://github.com/johanbrandhorst))
- Populate swagger method parameter description from message comments [\#692](https://github.com/grpc-ecosystem/grpc-gateway/pull/692) ([co3k](https://github.com/co3k))
- Updated doc and comments to reflect Permanent HTTP header keys prefixing [\#691](https://github.com/grpc-ecosystem/grpc-gateway/pull/691) ([crozzy](https://github.com/crozzy))
- protoc-gen-swagger: support JSON Schema Validation properties and add openapiv2\_field option [\#687](https://github.com/grpc-ecosystem/grpc-gateway/pull/687) ([co3k](https://github.com/co3k))
- Bazel expose protoc-gen-grpc-gateway [\#668](https://github.com/grpc-ecosystem/grpc-gateway/pull/668) ([afking](https://github.com/afking))
-  Fix protoc-gen-swagger to output gRPC method summary and descriptions as Swagger's them [\#667](https://github.com/grpc-ecosystem/grpc-gateway/pull/667) ([co3k](https://github.com/co3k))
- Allow explicit empty security definition to overwrite existing definitions [\#666](https://github.com/grpc-ecosystem/grpc-gateway/pull/666) ([co3k](https://github.com/co3k))
-  protoc-gen-swagger: Add ability to specify custom response objects  [\#663](https://github.com/grpc-ecosystem/grpc-gateway/pull/663) ([johanbrandhorst](https://github.com/johanbrandhorst))

## [v1.4.1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.4.1) (2018-05-23)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.4.0...v1.4.1)

**Closed issues:**

- Next release ? [\#605](https://github.com/grpc-ecosystem/grpc-gateway/issues/605)

**Merged pull requests:**

- Generate release notes for v1.4.1 [\#659](https://github.com/grpc-ecosystem/grpc-gateway/pull/659) ([achew22](https://github.com/achew22))
- Translate gRPC FailedPrecondition as HTTP PreconditionFailed [\#657](https://github.com/grpc-ecosystem/grpc-gateway/pull/657) ([slomek](https://github.com/slomek))

## [v1.4.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.4.0) (2018-05-20)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.3.1...v1.4.0)

**Implemented enhancements:**

- customize the error return [\#405](https://github.com/grpc-ecosystem/grpc-gateway/issues/405)
- Support map type in query string [\#316](https://github.com/grpc-ecosystem/grpc-gateway/issues/316)
- gRPC gateway Bazel build rules [\#66](https://github.com/grpc-ecosystem/grpc-gateway/issues/66)
- Support bytes fields in path parameter [\#5](https://github.com/grpc-ecosystem/grpc-gateway/issues/5)

**Closed issues:**

- the protoc\_gen\_swagger bazel rule generates non working import path. [\#633](https://github.com/grpc-ecosystem/grpc-gateway/issues/633)
- code.NotFound should return a 404 instead of a 405 [\#630](https://github.com/grpc-ecosystem/grpc-gateway/issues/630)
- field in query path not found [\#629](https://github.com/grpc-ecosystem/grpc-gateway/issues/629)
- how to use client pool in the gateway? [\#612](https://github.com/grpc-ecosystem/grpc-gateway/issues/612)
- pass http request uri to grpc server [\#587](https://github.com/grpc-ecosystem/grpc-gateway/issues/587)
- bidi streams have racy read caused by goroutine that closes over local variable [\#583](https://github.com/grpc-ecosystem/grpc-gateway/issues/583)
- Streamed response is not valid json \(or: is this the expected format?\) [\#581](https://github.com/grpc-ecosystem/grpc-gateway/issues/581)
- Import "google/api/annotations.proto" was not found or had errors. [\#574](https://github.com/grpc-ecosystem/grpc-gateway/issues/574)
- is there has a way to let grpc-gateway server support multiple endpoints [\#573](https://github.com/grpc-ecosystem/grpc-gateway/issues/573)
- would it be possible to avoid vendoring "third\_party/googleapis/" [\#572](https://github.com/grpc-ecosystem/grpc-gateway/issues/572)
- Is there anyway to output the access log of grpc gateway [\#556](https://github.com/grpc-ecosystem/grpc-gateway/issues/556)
- proto: no slice oenc for \*reflect.rtype = \[\]\*reflect.rtype [\#551](https://github.com/grpc-ecosystem/grpc-gateway/issues/551)
- autoreconf not found [\#549](https://github.com/grpc-ecosystem/grpc-gateway/issues/549)
- \[feature\]combine expvar into grpc-gateway [\#542](https://github.com/grpc-ecosystem/grpc-gateway/issues/542)
- Source code still imports "golang.org/x/net/context" [\#533](https://github.com/grpc-ecosystem/grpc-gateway/issues/533)
- Incorrect error message when execute protoc-gen-grpc-gateway to HTTP GET method with BODY [\#531](https://github.com/grpc-ecosystem/grpc-gateway/issues/531)
- add support for the google.api.HttpBody proto as a request [\#528](https://github.com/grpc-ecosystem/grpc-gateway/issues/528)
- Prefixed model names in generated swagger spec [\#525](https://github.com/grpc-ecosystem/grpc-gateway/issues/525)
- Better format for error.message in stream [\#519](https://github.com/grpc-ecosystem/grpc-gateway/issues/519)
- Getting this on go get . in the src directory: HelloService.pb.go:20:8 - no Go files in \go\src\google\api [\#518](https://github.com/grpc-ecosystem/grpc-gateway/issues/518)
- ci: set up codecov [\#513](https://github.com/grpc-ecosystem/grpc-gateway/issues/513)
- protoc-gen-swagger not using description field of info swagger object [\#511](https://github.com/grpc-ecosystem/grpc-gateway/issues/511)
- Cut a minor release for https://github.com/grpc-ecosystem/grpc-gateway/issues/495 [\#506](https://github.com/grpc-ecosystem/grpc-gateway/issues/506)
- bug: uncapitalized service name causes runtime error unknown function in service.pb.gw.go [\#484](https://github.com/grpc-ecosystem/grpc-gateway/issues/484)
- RESOURCE\_EXHAUSTED -\> 503 [\#431](https://github.com/grpc-ecosystem/grpc-gateway/issues/431)
- Adding authentication definitions to generated swagger files [\#428](https://github.com/grpc-ecosystem/grpc-gateway/issues/428)
- Move to stdlib context over x/net/context [\#326](https://github.com/grpc-ecosystem/grpc-gateway/issues/326)
- deprecate 1.6 and embrace \(\*http.Request\).Context by default [\#313](https://github.com/grpc-ecosystem/grpc-gateway/issues/313)

**Merged pull requests:**

- Generate a single swagger definition on demand [\#658](https://github.com/grpc-ecosystem/grpc-gateway/pull/658) ([achew22](https://github.com/achew22))
- Regenerate example files [\#656](https://github.com/grpc-ecosystem/grpc-gateway/pull/656) ([achew22](https://github.com/achew22))
- Add v1.4.0 changelog [\#655](https://github.com/grpc-ecosystem/grpc-gateway/pull/655) ([achew22](https://github.com/achew22))
- Replace the deprecated grpclog.Printf with grpclog.Infof [\#654](https://github.com/grpc-ecosystem/grpc-gateway/pull/654) ([a-robinson](https://github.com/a-robinson))
- Add README.md for examples [\#645](https://github.com/grpc-ecosystem/grpc-gateway/pull/645) ([liukgg](https://github.com/liukgg))
- JSONPb marshaler panics if input is nil interface [\#639](https://github.com/grpc-ecosystem/grpc-gateway/pull/639) ([jhump](https://github.com/jhump))
- provide access to underlying \*json.Decoder from JSONPb.NewDecoder [\#637](https://github.com/grpc-ecosystem/grpc-gateway/pull/637) ([jhump](https://github.com/jhump))
- fix compile errors caused by protobuf finally merging their dev branch to master [\#636](https://github.com/grpc-ecosystem/grpc-gateway/pull/636) ([jhump](https://github.com/jhump))
-  Generate import mappings. [\#635](https://github.com/grpc-ecosystem/grpc-gateway/pull/635) ([ensonic](https://github.com/ensonic))
- Add support for the grpc\_api\_configuration option in the bazel rule. [\#632](https://github.com/grpc-ecosystem/grpc-gateway/pull/632) ([ensonic](https://github.com/ensonic))
- Use repo relative labels in protoc-gen-swagger [\#631](https://github.com/grpc-ecosystem/grpc-gateway/pull/631) ([achew22](https://github.com/achew22))
- Correct dependencies in Makefile [\#626](https://github.com/grpc-ecosystem/grpc-gateway/pull/626) ([yugui](https://github.com/yugui))
- Avoid timing issues in the integration tests [\#624](https://github.com/grpc-ecosystem/grpc-gateway/pull/624) ([yugui](https://github.com/yugui))
- Fix typos in gRPC API Configuration usage documentation [\#623](https://github.com/grpc-ecosystem/grpc-gateway/pull/623) ([hacst](https://github.com/hacst))
- Skip unnecessary steps in USE\_BAZEL builds on TravisCI [\#622](https://github.com/grpc-ecosystem/grpc-gateway/pull/622) ([yugui](https://github.com/yugui))
- Support param for field from Oneof definition. [\#621](https://github.com/grpc-ecosystem/grpc-gateway/pull/621) ([bonafideyan](https://github.com/bonafideyan))
- Fixes file integrity errors on TravisCI [\#619](https://github.com/grpc-ecosystem/grpc-gateway/pull/619) ([yugui](https://github.com/yugui))
- Reorganize examples [\#618](https://github.com/grpc-ecosystem/grpc-gateway/pull/618) ([yugui](https://github.com/yugui))
- Update dependency declarations in the Makefile [\#617](https://github.com/grpc-ecosystem/grpc-gateway/pull/617) ([yugui](https://github.com/yugui))
- Support delete method in swagger generator [\#616](https://github.com/grpc-ecosystem/grpc-gateway/pull/616) ([blackdahila](https://github.com/blackdahila))
- feat\(bazel\): Add rule for generating .swagger.json files [\#613](https://github.com/grpc-ecosystem/grpc-gateway/pull/613) ([mrmeku](https://github.com/mrmeku))
- Support UNIX domain socket in the example servers [\#609](https://github.com/grpc-ecosystem/grpc-gateway/pull/609) ([yugui](https://github.com/yugui))
- misspelling [\#601](https://github.com/grpc-ecosystem/grpc-gateway/pull/601) ([chemidy](https://github.com/chemidy))
- Pulled out parseReq func into a generic package + tests [\#600](https://github.com/grpc-ecosystem/grpc-gateway/pull/600) ([f0rmiga](https://github.com/f0rmiga))
- Added Bazel support [\#599](https://github.com/grpc-ecosystem/grpc-gateway/pull/599) ([f0rmiga](https://github.com/f0rmiga))
- Add basic docs section [\#597](https://github.com/grpc-ecosystem/grpc-gateway/pull/597) ([achew22](https://github.com/achew22))
- Upgrade to go1.10 and regenerate [\#596](https://github.com/grpc-ecosystem/grpc-gateway/pull/596) ([achew22](https://github.com/achew22))
- Support cases where the request is done with transfer-encoding chunked [\#589](https://github.com/grpc-ecosystem/grpc-gateway/pull/589) ([jacksontj](https://github.com/jacksontj))
- Support multiple metadata annotators [\#586](https://github.com/grpc-ecosystem/grpc-gateway/pull/586) ([dmacthedestroyer](https://github.com/dmacthedestroyer))
- Changed to use more appropriate http status code for ResourceExhausted [\#580](https://github.com/grpc-ecosystem/grpc-gateway/pull/580) ([eleniums](https://github.com/eleniums))
- Stop marshalling any.Any types unnecessarily. [\#577](https://github.com/grpc-ecosystem/grpc-gateway/pull/577) ([johanbrandhorst](https://github.com/johanbrandhorst))
- fix racy access of err variable [\#575](https://github.com/grpc-ecosystem/grpc-gateway/pull/575) ([jhump](https://github.com/jhump))
- option to tweak generated Register\* function names [\#571](https://github.com/grpc-ecosystem/grpc-gateway/pull/571) ([jhump](https://github.com/jhump))
- runtime: return 503 not 403 with ResourceExhausted. [\#569](https://github.com/grpc-ecosystem/grpc-gateway/pull/569) ([hexfusion](https://github.com/hexfusion))
- \[\]byte in query now uses base64.StdEncoding [\#565](https://github.com/grpc-ecosystem/grpc-gateway/pull/565) ([lucasvo](https://github.com/lucasvo))
- Add 3rd party rpc protos in order to have access to status and error [\#563](https://github.com/grpc-ecosystem/grpc-gateway/pull/563) ([rvegas](https://github.com/rvegas))
- Add details to stream error response [\#561](https://github.com/grpc-ecosystem/grpc-gateway/pull/561) ([johanbrandhorst](https://github.com/johanbrandhorst))
- fix noenc error by fixing Details error field [\#557](https://github.com/grpc-ecosystem/grpc-gateway/pull/557) ([srenatus](https://github.com/srenatus))
- error details: add @type key by switching to any.Any [\#553](https://github.com/grpc-ecosystem/grpc-gateway/pull/553) ([srenatus](https://github.com/srenatus))
- Add a FAQ [\#550](https://github.com/grpc-ecosystem/grpc-gateway/pull/550) ([achew22](https://github.com/achew22))
- Add security fields support to protoc-gen-swagger [\#547](https://github.com/grpc-ecosystem/grpc-gateway/pull/547) ([ivucica](https://github.com/ivucica))
- Omit well-known type definitions from swagger output [\#541](https://github.com/grpc-ecosystem/grpc-gateway/pull/541) ([alexleigh](https://github.com/alexleigh))
- Use importPath to set package name rather than package path. [\#537](https://github.com/grpc-ecosystem/grpc-gateway/pull/537) ([rwlincoln](https://github.com/rwlincoln))
- Support for map type in query string [\#535](https://github.com/grpc-ecosystem/grpc-gateway/pull/535) ([adamstruck](https://github.com/adamstruck))
- Fix error message in protoc-gen-grpc-gateway \(for \#531\) [\#532](https://github.com/grpc-ecosystem/grpc-gateway/pull/532) ([budougumi0617](https://github.com/budougumi0617))
- runtime: support FieldMask as query param [\#529](https://github.com/grpc-ecosystem/grpc-gateway/pull/529) ([glerchundi](https://github.com/glerchundi))
- Fix decoding empty request body [\#527](https://github.com/grpc-ecosystem/grpc-gateway/pull/527) ([syhpoon](https://github.com/syhpoon))
- Add description, summary and tags fields in operationObject \(swagger\) [\#526](https://github.com/grpc-ecosystem/grpc-gateway/pull/526) ([devnull-](https://github.com/devnull-))
- Converts the first letter of service name to uppercase [\#522](https://github.com/grpc-ecosystem/grpc-gateway/pull/522) ([thurt](https://github.com/thurt))
- Add support for basic gRPC API Configuration YAML files [\#521](https://github.com/grpc-ecosystem/grpc-gateway/pull/521) ([hacst](https://github.com/hacst))
- Fix travis to only difftest on go 1.9 [\#520](https://github.com/grpc-ecosystem/grpc-gateway/pull/520) ([achew22](https://github.com/achew22))
- add error details to error json [\#515](https://github.com/grpc-ecosystem/grpc-gateway/pull/515) ([srenatus](https://github.com/srenatus))
- ci: add codecov [\#514](https://github.com/grpc-ecosystem/grpc-gateway/pull/514) ([tmc](https://github.com/tmc))
- Generate "Description" and "TermsOfService" fields [\#512](https://github.com/grpc-ecosystem/grpc-gateway/pull/512) ([lukasmalkmus](https://github.com/lukasmalkmus))
- Release 1.3.1 [\#509](https://github.com/grpc-ecosystem/grpc-gateway/pull/509) ([tmc](https://github.com/tmc))
- Support mapping bytes to \[\]byte [\#489](https://github.com/grpc-ecosystem/grpc-gateway/pull/489) ([loderunner](https://github.com/loderunner))
- properly respect file flag for protoc-gen-swagger [\#293](https://github.com/grpc-ecosystem/grpc-gateway/pull/293) ([tmc](https://github.com/tmc))

## [v1.3.1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.3.1) (2017-12-23)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.3.0...v1.3.1)

**Implemented enhancements:**

- Support import\_path? [\#443](https://github.com/grpc-ecosystem/grpc-gateway/issues/443)

**Closed issues:**

- protoc-gen-swagger missing definition issue [\#504](https://github.com/grpc-ecosystem/grpc-gateway/issues/504)
- Are gateway metrics available? [\#498](https://github.com/grpc-ecosystem/grpc-gateway/issues/498)
- Backwards incompatible change to chunked encoding [\#495](https://github.com/grpc-ecosystem/grpc-gateway/issues/495)
- Map of list [\#493](https://github.com/grpc-ecosystem/grpc-gateway/issues/493)
- How to run `makefile` for this repo? [\#491](https://github.com/grpc-ecosystem/grpc-gateway/issues/491)
- all SubConns are in TransientFailure [\#490](https://github.com/grpc-ecosystem/grpc-gateway/issues/490)
- Appengine Standard Environment: "not an Appengine context" [\#487](https://github.com/grpc-ecosystem/grpc-gateway/issues/487)
- Enum Path Parameter to Swagger [\#486](https://github.com/grpc-ecosystem/grpc-gateway/issues/486)
- Should v1.3 be also tagged as v1.3.0? [\#483](https://github.com/grpc-ecosystem/grpc-gateway/issues/483)
- HTTP response is not correct json encoded if the grpc return stream of objects. [\#481](https://github.com/grpc-ecosystem/grpc-gateway/issues/481)
- Support JSON-RPCv2 [\#477](https://github.com/grpc-ecosystem/grpc-gateway/issues/477)
- Naming convention? [\#475](https://github.com/grpc-ecosystem/grpc-gateway/issues/475)
- Request context not being used [\#470](https://github.com/grpc-ecosystem/grpc-gateway/issues/470)
- Generate Swagger documentation [\#469](https://github.com/grpc-ecosystem/grpc-gateway/issues/469)
- Support Request | make: swagger-codegen: Command not found [\#468](https://github.com/grpc-ecosystem/grpc-gateway/issues/468)
- How do you generate a swagger yaml file instead of json? [\#467](https://github.com/grpc-ecosystem/grpc-gateway/issues/467)
- Add default support for proto over http [\#465](https://github.com/grpc-ecosystem/grpc-gateway/issues/465)
- Allow compiling the gateway code to a different go package [\#463](https://github.com/grpc-ecosystem/grpc-gateway/issues/463)
- support google.api.HttpBody [\#457](https://github.com/grpc-ecosystem/grpc-gateway/issues/457)
- \[swagger bug\] with google/protobuf/wrappers.proto [\#453](https://github.com/grpc-ecosystem/grpc-gateway/issues/453)
- The tensorflow serving support RESTful api：{"error":"json: cannot unmarshal object into Go value of type \[\]json.RawMessage","code":3} [\#444](https://github.com/grpc-ecosystem/grpc-gateway/issues/444)
- choose some return fields omit or  not omit by configure [\#439](https://github.com/grpc-ecosystem/grpc-gateway/issues/439)
- swagger title and version hardcoded [\#437](https://github.com/grpc-ecosystem/grpc-gateway/issues/437)
- Change the path though http header [\#424](https://github.com/grpc-ecosystem/grpc-gateway/issues/424)
- google/protobuf/descriptor.proto: File not found [\#422](https://github.com/grpc-ecosystem/grpc-gateway/issues/422)
- Output file will not compile if the .proto file does not contain a service with parameters in the url path [\#389](https://github.com/grpc-ecosystem/grpc-gateway/issues/389)
- Scaling support [\#381](https://github.com/grpc-ecosystem/grpc-gateway/issues/381)
- I cannot get the default value from client side [\#380](https://github.com/grpc-ecosystem/grpc-gateway/issues/380)
- Problem with Generated annotations.proto file [\#377](https://github.com/grpc-ecosystem/grpc-gateway/issues/377)
- Release 1.3.0 [\#357](https://github.com/grpc-ecosystem/grpc-gateway/issues/357)
- swagger: Unclear comments' parser behaviour [\#352](https://github.com/grpc-ecosystem/grpc-gateway/issues/352)
- Support semicolon syntax in go\_package protobuf option [\#341](https://github.com/grpc-ecosystem/grpc-gateway/issues/341)
- Add SOAP proxy [\#339](https://github.com/grpc-ecosystem/grpc-gateway/issues/339)
- Support combination of query params and body for POSTs with body: "\*" [\#234](https://github.com/grpc-ecosystem/grpc-gateway/issues/234)
- Interceptor [\#221](https://github.com/grpc-ecosystem/grpc-gateway/issues/221)

**Merged pull requests:**

- Add support for --Import\_path [\#507](https://github.com/grpc-ecosystem/grpc-gateway/pull/507) ([achew22](https://github.com/achew22))
- Fix \#504 Missing Definitions [\#505](https://github.com/grpc-ecosystem/grpc-gateway/pull/505) ([warmans](https://github.com/warmans))
- Maintain default delimiter of newline [\#497](https://github.com/grpc-ecosystem/grpc-gateway/pull/497) ([jacksontj](https://github.com/jacksontj))
- Fix gen-swagger to support more well known types [\#496](https://github.com/grpc-ecosystem/grpc-gateway/pull/496) ([shouichi](https://github.com/shouichi))
- Use golang/protobuf instead of gogo/protobuf [\#494](https://github.com/grpc-ecosystem/grpc-gateway/pull/494) ([shouichi](https://github.com/shouichi))
- Fix stream delimiters [\#488](https://github.com/grpc-ecosystem/grpc-gateway/pull/488) ([afking](https://github.com/afking))
- ForwardResponseStream status code errors [\#482](https://github.com/grpc-ecosystem/grpc-gateway/pull/482) ([afking](https://github.com/afking))
- protoc-gen-grpc-gateway: flip request\_context default to true [\#474](https://github.com/grpc-ecosystem/grpc-gateway/pull/474) ([srenatus](https://github.com/srenatus))
- grpc-gateway/generator: respect full package [\#462](https://github.com/grpc-ecosystem/grpc-gateway/pull/462) ([glerchundi](https://github.com/glerchundi))
- Add proto marshaller for proto-over-http [\#459](https://github.com/grpc-ecosystem/grpc-gateway/pull/459) ([MatthewDolan](https://github.com/MatthewDolan))

## [v1.3.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.3.0) (2017-11-03)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.3...v1.3.0)

## [v1.3](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.3) (2017-11-03)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.2.2...v1.3)

**Closed issues:**

- Extract basic auth from URL [\#480](https://github.com/grpc-ecosystem/grpc-gateway/issues/480)
- Lack of "google/protobuf/descriptor.proto" [\#476](https://github.com/grpc-ecosystem/grpc-gateway/issues/476)
- question: how to indicate whether call is through grpc gateway [\#456](https://github.com/grpc-ecosystem/grpc-gateway/issues/456)
- How to define this restful api using pb? [\#452](https://github.com/grpc-ecosystem/grpc-gateway/issues/452)
- how to output field as an array of json values? [\#449](https://github.com/grpc-ecosystem/grpc-gateway/issues/449)
- How do I override maxMsgSize? [\#445](https://github.com/grpc-ecosystem/grpc-gateway/issues/445)
- OpenAPI spec is generated with duplicated operation IDs. [\#442](https://github.com/grpc-ecosystem/grpc-gateway/issues/442)
- This process seems to generate conflicting code with go-micro [\#440](https://github.com/grpc-ecosystem/grpc-gateway/issues/440)
- any way to let int64 marshal to int not string? [\#438](https://github.com/grpc-ecosystem/grpc-gateway/issues/438)
- Support  streaming [\#435](https://github.com/grpc-ecosystem/grpc-gateway/issues/435)
- Update DO NOT EDIT header in generated files [\#433](https://github.com/grpc-ecosystem/grpc-gateway/issues/433)
- generate code use context not "golang.org/x/net/context" [\#430](https://github.com/grpc-ecosystem/grpc-gateway/issues/430)
- Replace \n with spaces in swagger definitions [\#426](https://github.com/grpc-ecosystem/grpc-gateway/issues/426)
- \[question\]Is there any example for  http headers process? [\#420](https://github.com/grpc-ecosystem/grpc-gateway/issues/420)
- Is there any way to support a multipart form request? [\#410](https://github.com/grpc-ecosystem/grpc-gateway/issues/410)
- Not able to pass allow\_delete\_body to protoc-gen-grpc-gateway. [\#402](https://github.com/grpc-ecosystem/grpc-gateway/issues/402)
- returned errors should conform to google.rpc.Status [\#399](https://github.com/grpc-ecosystem/grpc-gateway/issues/399)
- Is there any way to generate python gateway code? [\#398](https://github.com/grpc-ecosystem/grpc-gateway/issues/398)
- how to handle arbitrary \(json\) structs [\#395](https://github.com/grpc-ecosystem/grpc-gateway/issues/395)
- \[question\]can give a url with query sting demo? [\#394](https://github.com/grpc-ecosystem/grpc-gateway/issues/394)
- \[question\]the swagger url generated is what? [\#393](https://github.com/grpc-ecosystem/grpc-gateway/issues/393)
- \[Question\] How do I use semantic versions? [\#392](https://github.com/grpc-ecosystem/grpc-gateway/issues/392)
- \[question\]how to run examples? [\#391](https://github.com/grpc-ecosystem/grpc-gateway/issues/391)
- Why does gateway use ServerMetadata? [\#388](https://github.com/grpc-ecosystem/grpc-gateway/issues/388)
- Can't generate code with last version [\#384](https://github.com/grpc-ecosystem/grpc-gateway/issues/384)
- is it ready for production use? [\#382](https://github.com/grpc-ecosystem/grpc-gateway/issues/382)
- Support Google Flatbuffers [\#376](https://github.com/grpc-ecosystem/grpc-gateway/issues/376)
- calling Enum by string name in requests using gogo/protobuf results in error. [\#372](https://github.com/grpc-ecosystem/grpc-gateway/issues/372)
- Definitions containing URLs with trailing slashes won't compile [\#370](https://github.com/grpc-ecosystem/grpc-gateway/issues/370)
- Should metadata annotator include the headers from incoming matcher? [\#368](https://github.com/grpc-ecosystem/grpc-gateway/issues/368)
-  metadata.NewOutgoingContext is undefined [\#364](https://github.com/grpc-ecosystem/grpc-gateway/issues/364)
- Why does not gateway forward headers as-is? [\#311](https://github.com/grpc-ecosystem/grpc-gateway/issues/311)
- Question: Why passing context to RegisterMyServiceHandler is required?  [\#301](https://github.com/grpc-ecosystem/grpc-gateway/issues/301)
- Allow whitelisting of particular HTTP headers to map to metadata. [\#253](https://github.com/grpc-ecosystem/grpc-gateway/issues/253)
- Swagger definitions don't handle parameters that are not explicitly required in the url [\#159](https://github.com/grpc-ecosystem/grpc-gateway/issues/159)

**Merged pull requests:**

- Fix wrong method names [\#603](https://github.com/grpc-ecosystem/grpc-gateway/pull/603) ([yugui](https://github.com/yugui))
- Streaming forward handler fix chunk encoding [\#479](https://github.com/grpc-ecosystem/grpc-gateway/pull/479) ([afking](https://github.com/afking))
- Fix logic handling primitive wrapper in URL params [\#478](https://github.com/grpc-ecosystem/grpc-gateway/pull/478) ([tgeng](https://github.com/tgeng))
- runtime: use r.Context\(\) [\#473](https://github.com/grpc-ecosystem/grpc-gateway/pull/473) ([srenatus](https://github.com/srenatus))
- Optional SourceCodeInfo [\#466](https://github.com/grpc-ecosystem/grpc-gateway/pull/466) ([afking](https://github.com/afking))
- Some steps to fix Travis CI [\#461](https://github.com/grpc-ecosystem/grpc-gateway/pull/461) ([AlekSi](https://github.com/AlekSi))
- fix 2 typos in Registry.SetPrefix's comment [\#455](https://github.com/grpc-ecosystem/grpc-gateway/pull/455) ([hectorj](https://github.com/hectorj))
- Add Handler method to pass in client [\#454](https://github.com/grpc-ecosystem/grpc-gateway/pull/454) ([jacksontj](https://github.com/jacksontj))
- Fallback to JSON name when matching URL parameter. [\#450](https://github.com/grpc-ecosystem/grpc-gateway/pull/450) ([tgeng](https://github.com/tgeng))
- Update DO NOT EDIT template. [\#434](https://github.com/grpc-ecosystem/grpc-gateway/pull/434) ([AlekSi](https://github.com/AlekSi))
- Memoise calls to fullyQualifiedNameToSwaggerName to speed it up for large registries [\#421](https://github.com/grpc-ecosystem/grpc-gateway/pull/421) ([peterebden](https://github.com/peterebden))
- Update Swagger Codegen from 2.1.6 to 2.2.2 [\#415](https://github.com/grpc-ecosystem/grpc-gateway/pull/415) ([yugui](https://github.com/yugui))
- Return codes.InvalidArgument to rather return HTTP 400 instead of HTTP 500 [\#409](https://github.com/grpc-ecosystem/grpc-gateway/pull/409) ([vaporz](https://github.com/vaporz))
- improve {incoming,outgoing}HeaderMatcher logic [\#408](https://github.com/grpc-ecosystem/grpc-gateway/pull/408) ([flisky](https://github.com/flisky))
- improve WKT handling in gateway and openapi output [\#404](https://github.com/grpc-ecosystem/grpc-gateway/pull/404) ([tmc](https://github.com/tmc))
- Return if runtime.AnnotateContext gave error [\#403](https://github.com/grpc-ecosystem/grpc-gateway/pull/403) ([tamalsaha](https://github.com/tamalsaha))
- jsonpb: update tests to reflect new jsonpb behavior [\#401](https://github.com/grpc-ecosystem/grpc-gateway/pull/401) ([tmc](https://github.com/tmc))
- Reference import grpc Status to suppress unused errors. [\#387](https://github.com/grpc-ecosystem/grpc-gateway/pull/387) ([tamalsaha](https://github.com/tamalsaha))
- ci: regen with current protoc-gen-go [\#385](https://github.com/grpc-ecosystem/grpc-gateway/pull/385) ([tmc](https://github.com/tmc))
- Use status package for error and introduce WithProtoErrorHandler option [\#378](https://github.com/grpc-ecosystem/grpc-gateway/pull/378) ([kazegusuri](https://github.com/kazegusuri))
- Return response headers from grpc server [\#374](https://github.com/grpc-ecosystem/grpc-gateway/pull/374) ([tamalsaha](https://github.com/tamalsaha))
- Skip unreferenced messages in definitions. [\#371](https://github.com/grpc-ecosystem/grpc-gateway/pull/371) ([lantame](https://github.com/lantame))
- Use canonical header form in default header matcher. [\#369](https://github.com/grpc-ecosystem/grpc-gateway/pull/369) ([tamalsaha](https://github.com/tamalsaha))
- support allow\_delete\_body for protoc-gen-grpc-gateway [\#318](https://github.com/grpc-ecosystem/grpc-gateway/pull/318) ([flisky](https://github.com/flisky))
- fixes package name override doesn't work [\#277](https://github.com/grpc-ecosystem/grpc-gateway/pull/277) ([favadi](https://github.com/favadi))
- add custom options to allow more control of swagger/openapi output [\#145](https://github.com/grpc-ecosystem/grpc-gateway/pull/145) ([ivucica](https://github.com/ivucica))

## [v1.2.2](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.2.2) (2017-04-17)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.2.1...v1.2.2)

**Merged pull requests:**

- Add changelog for 1.2.2 [\#363](https://github.com/grpc-ecosystem/grpc-gateway/pull/363) ([tmc](https://github.com/tmc))
- metadata: fix properly and change to Outgoing [\#361](https://github.com/grpc-ecosystem/grpc-gateway/pull/361) ([tmc](https://github.com/tmc))

## [v1.2.1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.2.1) (2017-04-17)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.2.0...v1.2.1)

**Fixed bugs:**

- reflect upstream grpc metadata api change [\#358](https://github.com/grpc-ecosystem/grpc-gateway/issues/358)

**Closed issues:**

- Empty value omitted [\#355](https://github.com/grpc-ecosystem/grpc-gateway/issues/355)
- Must generate reverse proxy in same package? [\#353](https://github.com/grpc-ecosystem/grpc-gateway/issues/353)
- Release 1.2.0 [\#340](https://github.com/grpc-ecosystem/grpc-gateway/issues/340)
- Cut another release [\#278](https://github.com/grpc-ecosystem/grpc-gateway/issues/278)

**Merged pull requests:**

- Add changelog for 1.2.1 [\#360](https://github.com/grpc-ecosystem/grpc-gateway/pull/360) ([tmc](https://github.com/tmc))
- bugfix: reflect upstream api change. [\#359](https://github.com/grpc-ecosystem/grpc-gateway/pull/359) ([tmc](https://github.com/tmc))

## [v1.2.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.2.0) (2017-03-31)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.2.0.rc1...v1.2.0)

**Closed issues:**

- Problem with \*.proto as "no buildable Go source files" [\#338](https://github.com/grpc-ecosystem/grpc-gateway/issues/338)
- Invalid import during code generation [\#337](https://github.com/grpc-ecosystem/grpc-gateway/issues/337)

**Merged pull requests:**

- Add changelog for 1.2.0 [\#342](https://github.com/grpc-ecosystem/grpc-gateway/pull/342) ([tmc](https://github.com/tmc))

## [v1.2.0.rc1](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.2.0.rc1) (2017-03-24)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.1.0...v1.2.0.rc1)

**Implemented enhancements:**

- Support for Any types [\#80](https://github.com/grpc-ecosystem/grpc-gateway/issues/80)
- improve\(genswagger:template\):added support for google.protobuf.Timestamp [\#209](https://github.com/grpc-ecosystem/grpc-gateway/pull/209) ([EranAvidor](https://github.com/EranAvidor))

**Fixed bugs:**

- Support for multi-segment elements [\#122](https://github.com/grpc-ecosystem/grpc-gateway/issues/122)

**Closed issues:**

- Go get breaks with autogenerated code [\#331](https://github.com/grpc-ecosystem/grpc-gateway/issues/331)
- Fresh install no longer generates necessary `google/api/annotations.pb.go` & `google/api/http.pb.go` files. [\#327](https://github.com/grpc-ecosystem/grpc-gateway/issues/327)
- Panic with query parameters [\#324](https://github.com/grpc-ecosystem/grpc-gateway/issues/324)
- Swagger-UI query parameters for enum types are sent as strings [\#320](https://github.com/grpc-ecosystem/grpc-gateway/issues/320)
- hide the object name in the response [\#317](https://github.com/grpc-ecosystem/grpc-gateway/issues/317)
- Package imported but not used [\#310](https://github.com/grpc-ecosystem/grpc-gateway/issues/310)
- Authorization headers aren't specified in Swagger.json [\#309](https://github.com/grpc-ecosystem/grpc-gateway/issues/309)
- Generating swagger version, contact name etc in generated docs [\#303](https://github.com/grpc-ecosystem/grpc-gateway/issues/303)
- Feature request: custom content type per service and rpc [\#302](https://github.com/grpc-ecosystem/grpc-gateway/issues/302)
- Reference: another RESTful api-gateway [\#299](https://github.com/grpc-ecosystem/grpc-gateway/issues/299)
- Integration with other languages is partially broken [\#298](https://github.com/grpc-ecosystem/grpc-gateway/issues/298)
- jsonpb convert int64 to integer instead of string [\#296](https://github.com/grpc-ecosystem/grpc-gateway/issues/296)
- default enum value is omitted [\#294](https://github.com/grpc-ecosystem/grpc-gateway/issues/294)
- Advice: could we simplify the flow as the below [\#292](https://github.com/grpc-ecosystem/grpc-gateway/issues/292)
- examples/browser test failure: TypeError: undefined is not a function \(evaluating 'window.location.protocol.startsWith\('chrome-extension'\)'\) [\#287](https://github.com/grpc-ecosystem/grpc-gateway/issues/287)
- ./entrypoint.go:25: undefined: api.RegisterYourServiceHandlerFromEndpoint [\#285](https://github.com/grpc-ecosystem/grpc-gateway/issues/285)
- Query params not handled in swagger file [\#284](https://github.com/grpc-ecosystem/grpc-gateway/issues/284)
- Please help: google/api/annotations.proto: File not found. [\#283](https://github.com/grpc-ecosystem/grpc-gateway/issues/283)
- Option to Allow Swagger for DELETEs with a body [\#279](https://github.com/grpc-ecosystem/grpc-gateway/issues/279)
- client declared and not used compilation error, after recent upgrade [\#276](https://github.com/grpc-ecosystem/grpc-gateway/issues/276)
- feature request / idea: generating JSONRPC2 client proxies from GRPC [\#272](https://github.com/grpc-ecosystem/grpc-gateway/issues/272)
- protoc-swagger-generator messes up the comments if there is rpc method that does not have rest [\#263](https://github.com/grpc-ecosystem/grpc-gateway/issues/263)
- Swagger Gen: underscores -\> lowerCamelCase field names and refs [\#261](https://github.com/grpc-ecosystem/grpc-gateway/issues/261)
- Timestamp as URL param causes bad request error [\#260](https://github.com/grpc-ecosystem/grpc-gateway/issues/260)
- "proto: no coders for int" printed whenever a gRPC error is returned over grpc-gateway. [\#259](https://github.com/grpc-ecosystem/grpc-gateway/issues/259)
- Compatibility with grpc.SupportPackageIsVersion4 [\#258](https://github.com/grpc-ecosystem/grpc-gateway/issues/258)
- How to use circuit breaker in this grpc gateway? [\#257](https://github.com/grpc-ecosystem/grpc-gateway/issues/257)
- cannot use example code to generate [\#255](https://github.com/grpc-ecosystem/grpc-gateway/issues/255)
- tests fail on go tip due to importing of main packages in test [\#250](https://github.com/grpc-ecosystem/grpc-gateway/issues/250)
- Add NGINX support [\#249](https://github.com/grpc-ecosystem/grpc-gateway/issues/249)
- Error when reverse proxy to gRPC server \(which is impl with Node.js\) [\#246](https://github.com/grpc-ecosystem/grpc-gateway/issues/246)
- Error output titlecase instead of lowercase [\#243](https://github.com/grpc-ecosystem/grpc-gateway/issues/243)
- Option field "\(google.api.http\)" is not a field or extension of message "ServiceOptions" [\#241](https://github.com/grpc-ecosystem/grpc-gateway/issues/241)
- Implement credentials handler in-box [\#238](https://github.com/grpc-ecosystem/grpc-gateway/issues/238)
- Proposal: Support WKT structs for URL params [\#237](https://github.com/grpc-ecosystem/grpc-gateway/issues/237)
- Example of /} in path template [\#232](https://github.com/grpc-ecosystem/grpc-gateway/issues/232)
- Serving swagger.json from runtime mux? [\#230](https://github.com/grpc-ecosystem/grpc-gateway/issues/230)
- ETCDclientv3 build error with the latest changes - github.com/grpc-ecosystem/grpc-gateway/runtime/marshal\_jsonpb.go:114: undefined: jsonpb.Unmarshaler [\#226](https://github.com/grpc-ecosystem/grpc-gateway/issues/226)
- Map in GET request [\#223](https://github.com/grpc-ecosystem/grpc-gateway/issues/223)
- HTTPS no longer works [\#220](https://github.com/grpc-ecosystem/grpc-gateway/issues/220)
- --swagger\_out plugin translates proto type int64 to string in Swagger specification [\#219](https://github.com/grpc-ecosystem/grpc-gateway/issues/219)
- Response body as a single field [\#217](https://github.com/grpc-ecosystem/grpc-gateway/issues/217)
- documentation of semantics of endpoint declarations [\#212](https://github.com/grpc-ecosystem/grpc-gateway/issues/212)
- gen-swagger does not generate PATCH method endpoints [\#211](https://github.com/grpc-ecosystem/grpc-gateway/issues/211)
- protoc-gen-grpc-gateway doesn't work correctly with option go\_package [\#207](https://github.com/grpc-ecosystem/grpc-gateway/issues/207)
- Browser Side Streaming Best Practices [\#206](https://github.com/grpc-ecosystem/grpc-gateway/issues/206)
- Does grpc-gateway support App Engine? [\#204](https://github.com/grpc-ecosystem/grpc-gateway/issues/204)
- "use of internal package" error, after moving to grpc-ecosystem [\#203](https://github.com/grpc-ecosystem/grpc-gateway/issues/203)
- Move to google.golang.org/genproto instead of shipping annotations.proto. [\#202](https://github.com/grpc-ecosystem/grpc-gateway/issues/202)
- Release v1.1.0 [\#196](https://github.com/grpc-ecosystem/grpc-gateway/issues/196)
- marshaler runtime.Marshaler does not handle io.EOF when decoding [\#195](https://github.com/grpc-ecosystem/grpc-gateway/issues/195)
- protobuf enumerated values now returned as strings instead of numbers. [\#186](https://github.com/grpc-ecosystem/grpc-gateway/issues/186)
- support annotating fields as required \(in swagger/oapi generation\)? [\#175](https://github.com/grpc-ecosystem/grpc-gateway/issues/175)
- architectural question: Can i codegen the client code for talking to the server ? [\#167](https://github.com/grpc-ecosystem/grpc-gateway/issues/167)
- Passing ENUM value as URL parameter throws error [\#166](https://github.com/grpc-ecosystem/grpc-gateway/issues/166)
- Support specifying which schemes should be output in swagger.json [\#161](https://github.com/grpc-ecosystem/grpc-gateway/issues/161)
- Use headers for routing [\#157](https://github.com/grpc-ecosystem/grpc-gateway/issues/157)
- ENUM in swagger.json makes client code failed to parse response from gateway [\#153](https://github.com/grpc-ecosystem/grpc-gateway/issues/153)
- Support map types [\#140](https://github.com/grpc-ecosystem/grpc-gateway/issues/140)
- generate OpenAPI/swagger documentation at run time? [\#138](https://github.com/grpc-ecosystem/grpc-gateway/issues/138)
- After the 1.7 release, update .travis.yaml to check the compiled proto output [\#137](https://github.com/grpc-ecosystem/grpc-gateway/issues/137)
- Getting parsed runtime.Pattern from server mux [\#127](https://github.com/grpc-ecosystem/grpc-gateway/issues/127)
- REST API without proxying [\#46](https://github.com/grpc-ecosystem/grpc-gateway/issues/46)

**Merged pull requests:**

- Remove an obsolete custom option [\#604](https://github.com/grpc-ecosystem/grpc-gateway/pull/604) ([yugui](https://github.com/yugui))
- Support user configurable header forwarding & context metadata [\#336](https://github.com/grpc-ecosystem/grpc-gateway/pull/336) ([tamalsaha](https://github.com/tamalsaha))
- Update go\_out parameter to remove comma [\#333](https://github.com/grpc-ecosystem/grpc-gateway/pull/333) ([tmc](https://github.com/tmc))
- Update stale path in README [\#332](https://github.com/grpc-ecosystem/grpc-gateway/pull/332) ([tmc](https://github.com/tmc))
- improve documentation regarding external dependencies [\#330](https://github.com/grpc-ecosystem/grpc-gateway/pull/330) ([CaptTofu](https://github.com/CaptTofu))
- Return an error on invalid nested query parameters. [\#329](https://github.com/grpc-ecosystem/grpc-gateway/pull/329) ([fische](https://github.com/fische))
- Update upstream proto files and add google.golang.org/genproto support. [\#325](https://github.com/grpc-ecosystem/grpc-gateway/pull/325) ([tmc](https://github.com/tmc))
- Support oneof fields in query params [\#321](https://github.com/grpc-ecosystem/grpc-gateway/pull/321) ([nilium](https://github.com/nilium))
- Do not ignore the error coming from http.ListenAndServe in examples [\#319](https://github.com/grpc-ecosystem/grpc-gateway/pull/319) ([campoy](https://github.com/campoy))
- Look up enum value maps by their proto name [\#315](https://github.com/grpc-ecosystem/grpc-gateway/pull/315) ([nilium](https://github.com/nilium))
- enable parsing enums from query parameters [\#314](https://github.com/grpc-ecosystem/grpc-gateway/pull/314) ([tzneal](https://github.com/tzneal))
- Do not add imports from methods with no bindings. [\#312](https://github.com/grpc-ecosystem/grpc-gateway/pull/312) ([fische](https://github.com/fische))
- Convert the first letter of method name to upper [\#300](https://github.com/grpc-ecosystem/grpc-gateway/pull/300) ([lipixun](https://github.com/lipixun))
- write query parameters to swagger definition [\#297](https://github.com/grpc-ecosystem/grpc-gateway/pull/297) ([t-yuki](https://github.com/t-yuki))
- Bump swagger-client to 2.1.28 for examples/browser [\#290](https://github.com/grpc-ecosystem/grpc-gateway/pull/290) ([tmc](https://github.com/tmc))
- pin to version before es6ism [\#289](https://github.com/grpc-ecosystem/grpc-gateway/pull/289) ([tmc](https://github.com/tmc))
- Prevent lack of http bindings from generating non-building output [\#286](https://github.com/grpc-ecosystem/grpc-gateway/pull/286) ([tmc](https://github.com/tmc))
- Added support for Timestamp in URL. [\#281](https://github.com/grpc-ecosystem/grpc-gateway/pull/281) ([johansja](https://github.com/johansja))
- add plugin param 'allow\_delete\_body'  [\#280](https://github.com/grpc-ecosystem/grpc-gateway/pull/280) ([msample](https://github.com/msample))
- Fix ruby gen command [\#275](https://github.com/grpc-ecosystem/grpc-gateway/pull/275) ([bluehallu](https://github.com/bluehallu))
- Make grpc-gateway support enum fields in path parameter [\#273](https://github.com/grpc-ecosystem/grpc-gateway/pull/273) ([linuxerwang](https://github.com/linuxerwang))
- remove unnecessary make\(\) [\#271](https://github.com/grpc-ecosystem/grpc-gateway/pull/271) ([tmc](https://github.com/tmc))
- preserve field order in swagger spec [\#270](https://github.com/grpc-ecosystem/grpc-gateway/pull/270) ([tmc](https://github.com/tmc))
- Merge \#228 [\#268](https://github.com/grpc-ecosystem/grpc-gateway/pull/268) ([tmc](https://github.com/tmc))
- Handle methods with no bindings more carefully [\#267](https://github.com/grpc-ecosystem/grpc-gateway/pull/267) ([tmc](https://github.com/tmc))
- describe default marshaler in README.md [\#266](https://github.com/grpc-ecosystem/grpc-gateway/pull/266) ([tmc](https://github.com/tmc))
- Add request\_context flag to utilize \(\*http.Request\).Context\(\) in handlers [\#265](https://github.com/grpc-ecosystem/grpc-gateway/pull/265) ([tmc](https://github.com/tmc))
- Regenerate examples [\#264](https://github.com/grpc-ecosystem/grpc-gateway/pull/264) ([tmc](https://github.com/tmc))
- Correct runtime.errorBody protobuf field tag [\#256](https://github.com/grpc-ecosystem/grpc-gateway/pull/256) ([tmc](https://github.com/tmc))
- Pass permanent HTTP request headers [\#252](https://github.com/grpc-ecosystem/grpc-gateway/pull/252) ([tmc](https://github.com/tmc))
- regenerate examples, fix tests for go tip [\#248](https://github.com/grpc-ecosystem/grpc-gateway/pull/248) ([tmc](https://github.com/tmc))
- Render the swagger request body properly [\#247](https://github.com/grpc-ecosystem/grpc-gateway/pull/247) ([dprotaso](https://github.com/dprotaso))
- Error output should have lowercase attribute names [\#244](https://github.com/grpc-ecosystem/grpc-gateway/pull/244) ([nathanborror](https://github.com/nathanborror))
- runtime - export prefix constants [\#236](https://github.com/grpc-ecosystem/grpc-gateway/pull/236) ([philipithomas](https://github.com/philipithomas))
- README - Add CoreOS example [\#231](https://github.com/grpc-ecosystem/grpc-gateway/pull/231) ([philipithomas](https://github.com/philipithomas))
- Docs - Add section about how HTTP maps to gRPC [\#227](https://github.com/grpc-ecosystem/grpc-gateway/pull/227) ([philipithomas](https://github.com/philipithomas))
- readme: added links to additional documentation [\#222](https://github.com/grpc-ecosystem/grpc-gateway/pull/222) ([sdemos](https://github.com/sdemos))
- Use a released version of protoc [\#216](https://github.com/grpc-ecosystem/grpc-gateway/pull/216) ([yugui](https://github.com/yugui))
- Add contribution guideline [\#210](https://github.com/grpc-ecosystem/grpc-gateway/pull/210) ([yugui](https://github.com/yugui))
- Allowing unknown fields to be dropped instead of returning error from… [\#208](https://github.com/grpc-ecosystem/grpc-gateway/pull/208) ([sriniven](https://github.com/sriniven))
- Avoid Internal Server Error on zero-length input for bidi streaming [\#200](https://github.com/grpc-ecosystem/grpc-gateway/pull/200) ([yugui](https://github.com/yugui))

## [v1.1.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.1.0) (2016-07-23)
[Full Changelog](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.0.0...v1.1.0)

**Implemented enhancements:**

- Support oneof types of fields [\#82](https://github.com/grpc-ecosystem/grpc-gateway/issues/82)
- allow use of jsonpb for marshaling [\#79](https://github.com/grpc-ecosystem/grpc-gateway/issues/79)

**Closed issues:**

- Generating a gRPC stub using Gateway generates a gRPC internal error [\#198](https://github.com/grpc-ecosystem/grpc-gateway/issues/198)
- Build fails with error: use of internal package not allowed [\#197](https://github.com/grpc-ecosystem/grpc-gateway/issues/197)
- google/protobuf/descriptor.proto: File not found. [\#194](https://github.com/grpc-ecosystem/grpc-gateway/issues/194)
- please tag releases [\#189](https://github.com/grpc-ecosystem/grpc-gateway/issues/189)
- Support for path collapsing for embedded structs? [\#187](https://github.com/grpc-ecosystem/grpc-gateway/issues/187)
- \[ACTION Required\] Moving to grpc-ecosystem [\#179](https://github.com/grpc-ecosystem/grpc-gateway/issues/179)
- Ading grpc-timeout support [\#107](https://github.com/grpc-ecosystem/grpc-gateway/issues/107)
- Generation of one swagger file out of multiple protos? [\#99](https://github.com/grpc-ecosystem/grpc-gateway/issues/99)

**Merged pull requests:**

- Rename packages to follow the repository transfer [\#192](https://github.com/grpc-ecosystem/grpc-gateway/pull/192) ([yugui](https://github.com/yugui))
- return err early if EOF to prevent logging in normal conditions [\#191](https://github.com/grpc-ecosystem/grpc-gateway/pull/191) ([tmc](https://github.com/tmc))
- send Trailer header on error [\#188](https://github.com/grpc-ecosystem/grpc-gateway/pull/188) ([kazegusuri](https://github.com/kazegusuri))
- generate swagger output for streaming endpoints with a basic note [\#183](https://github.com/grpc-ecosystem/grpc-gateway/pull/183) ([tmc](https://github.com/tmc))

## [v1.0.0](https://github.com/grpc-ecosystem/grpc-gateway/tree/v1.0.0) (2016-06-15)
**Implemented enhancements:**

- support protobuf-over-HTTP [\#124](https://github.com/grpc-ecosystem/grpc-gateway/issues/124)
- Static mapping from proto field names to golang field names [\#86](https://github.com/grpc-ecosystem/grpc-gateway/issues/86)
- Format Errors to JSON [\#25](https://github.com/grpc-ecosystem/grpc-gateway/issues/25)
- Emit API definition in Swagger schema format [\#9](https://github.com/grpc-ecosystem/grpc-gateway/issues/9)
- Method parameter in query string [\#6](https://github.com/grpc-ecosystem/grpc-gateway/issues/6)
- Integrate authentication [\#4](https://github.com/grpc-ecosystem/grpc-gateway/issues/4)
- Add swagger support [\#68](https://github.com/grpc-ecosystem/grpc-gateway/pull/68) ([achew22](https://github.com/achew22))
- Add runtime.WithForwardResponseOption [\#53](https://github.com/grpc-ecosystem/grpc-gateway/pull/53) ([pedgeio](https://github.com/pedgeio))

**Fixed bugs:**

- recent annotation change requires req.RemoteAddr to be populated [\#177](https://github.com/grpc-ecosystem/grpc-gateway/issues/177)
- Runtime panic with CloseNotify [\#115](https://github.com/grpc-ecosystem/grpc-gateway/issues/115)
- Gateway code generation broken when rpc method with a streaming response has an input paramter [\#35](https://github.com/grpc-ecosystem/grpc-gateway/issues/35)
- URL usage of nested messages causes nil pointer in proto3 [\#32](https://github.com/grpc-ecosystem/grpc-gateway/issues/32)
- Multiple .proto files generates invalid import statements. [\#22](https://github.com/grpc-ecosystem/grpc-gateway/issues/22)

**Closed issues:**

- remote peer address is lost in ctx - always resolves to localhost [\#173](https://github.com/grpc-ecosystem/grpc-gateway/issues/173)
- Bidirectional streams don't concurrently Send and Recv [\#169](https://github.com/grpc-ecosystem/grpc-gateway/issues/169)
- Error: failed to import google/api/annotations.proto [\#165](https://github.com/grpc-ecosystem/grpc-gateway/issues/165)
- Test datarace in controlapi [\#163](https://github.com/grpc-ecosystem/grpc-gateway/issues/163)
- not enough arguments in call to runtime.HTTPError [\#162](https://github.com/grpc-ecosystem/grpc-gateway/issues/162)
- String-values for Enums in request object are not recognized. [\#150](https://github.com/grpc-ecosystem/grpc-gateway/issues/150)
- Handling of import public "file.proto" [\#139](https://github.com/grpc-ecosystem/grpc-gateway/issues/139)
- Does grpc-gateway support http middleware? [\#132](https://github.com/grpc-ecosystem/grpc-gateway/issues/132)
- push to web clients using WS or SSE ? [\#131](https://github.com/grpc-ecosystem/grpc-gateway/issues/131)
- protoc-gen-swagger comment parsing for documentation gen [\#128](https://github.com/grpc-ecosystem/grpc-gateway/issues/128)
- generated code has a data race [\#123](https://github.com/grpc-ecosystem/grpc-gateway/issues/123)
- panic: net/http: CloseNotify called after ServeHTTP finished [\#121](https://github.com/grpc-ecosystem/grpc-gateway/issues/121)
- CloseNotify race with ServeHTTP [\#119](https://github.com/grpc-ecosystem/grpc-gateway/issues/119)
- echo service example does not compile [\#117](https://github.com/grpc-ecosystem/grpc-gateway/issues/117)
- go vet issues in template\_test.go [\#113](https://github.com/grpc-ecosystem/grpc-gateway/issues/113)
- undefined: proto.SizeVarint [\#103](https://github.com/grpc-ecosystem/grpc-gateway/issues/103)
- Closing the HTTP connection does not cancel the Context [\#101](https://github.com/grpc-ecosystem/grpc-gateway/issues/101)
- Logging [\#92](https://github.com/grpc-ecosystem/grpc-gateway/issues/92)
- Missing default values in JSON output? [\#91](https://github.com/grpc-ecosystem/grpc-gateway/issues/91)
- Better grpc error strings [\#87](https://github.com/grpc-ecosystem/grpc-gateway/issues/87)
- Fields aren't named in the same manner as golang/protobuf [\#84](https://github.com/grpc-ecosystem/grpc-gateway/issues/84)
- Header Forwarding from server. [\#73](https://github.com/grpc-ecosystem/grpc-gateway/issues/73)
- No pattern specified in google.api.HttpRule [\#70](https://github.com/grpc-ecosystem/grpc-gateway/issues/70)
- cannot find package "google/api" [\#67](https://github.com/grpc-ecosystem/grpc-gateway/issues/67)
- Generated .pb.go with services no longer works with latest version of grpc-go. [\#62](https://github.com/grpc-ecosystem/grpc-gateway/issues/62)
- JavaScript Proxy [\#61](https://github.com/grpc-ecosystem/grpc-gateway/issues/61)
- Add HTTP error code, error status to responseStreamChunk Error [\#58](https://github.com/grpc-ecosystem/grpc-gateway/issues/58)
- Reverse the code gen idea [\#44](https://github.com/grpc-ecosystem/grpc-gateway/issues/44)
- array of maps in json [\#43](https://github.com/grpc-ecosystem/grpc-gateway/issues/43)
- Examples break with 1.5 because of import of "main" examples package  [\#37](https://github.com/grpc-ecosystem/grpc-gateway/issues/37)
- Breaks with 1.5rc1 due to "internal" package name. [\#36](https://github.com/grpc-ecosystem/grpc-gateway/issues/36)
- Feature Request: Support for non-nullable nested messages. [\#20](https://github.com/grpc-ecosystem/grpc-gateway/issues/20)
- Is PascalFromSnake the right conversion to be doing? [\#19](https://github.com/grpc-ecosystem/grpc-gateway/issues/19)
- Infinite loop in generator when package name conflicts [\#17](https://github.com/grpc-ecosystem/grpc-gateway/issues/17)
- google.api.http options in multi-line format not supported [\#16](https://github.com/grpc-ecosystem/grpc-gateway/issues/16)
- Is there any plan to developing a C++ version? [\#15](https://github.com/grpc-ecosystem/grpc-gateway/issues/15)

**Merged pull requests:**

- Regenerate files with the latest protoc-gen-go [\#185](https://github.com/grpc-ecosystem/grpc-gateway/pull/185) ([yugui](https://github.com/yugui))
- Add browser examples [\#184](https://github.com/grpc-ecosystem/grpc-gateway/pull/184) ([yugui](https://github.com/yugui))
- Fix golint and go vet errors [\#182](https://github.com/grpc-ecosystem/grpc-gateway/pull/182) ([yugui](https://github.com/yugui))
- Add integration with clients generated by swagger-codegen [\#181](https://github.com/grpc-ecosystem/grpc-gateway/pull/181) ([yugui](https://github.com/yugui))
- Simplify example services [\#180](https://github.com/grpc-ecosystem/grpc-gateway/pull/180) ([yugui](https://github.com/yugui))
- Avoid errors when req.RemoteAddr is empty [\#178](https://github.com/grpc-ecosystem/grpc-gateway/pull/178) ([yugui](https://github.com/yugui))
- Feature/headers [\#176](https://github.com/grpc-ecosystem/grpc-gateway/pull/176) ([yugui](https://github.com/yugui))
- Include HTTP req.remoteAddr in gRPC ctx [\#174](https://github.com/grpc-ecosystem/grpc-gateway/pull/174) ([mikeatlas](https://github.com/mikeatlas))
- Update dependencies [\#171](https://github.com/grpc-ecosystem/grpc-gateway/pull/171) ([yugui](https://github.com/yugui))
- Add bidirectional streaming support by running Send\(\) and Recv\(\) concurrently [\#170](https://github.com/grpc-ecosystem/grpc-gateway/pull/170) ([tmc](https://github.com/tmc))
- make Authorization header check case-insensitive to comply with RFC 2616 4.2 [\#164](https://github.com/grpc-ecosystem/grpc-gateway/pull/164) ([tmc](https://github.com/tmc))
- jsonpb: avoid duplicating upstream's struct [\#158](https://github.com/grpc-ecosystem/grpc-gateway/pull/158) ([tamird](https://github.com/tamird))
- Generate Swagger description for service methods using proto comments. [\#156](https://github.com/grpc-ecosystem/grpc-gateway/pull/156) ([t-yuki](https://github.com/t-yuki))
- Implement gRPC timeout support for inbound HTTP headers [\#155](https://github.com/grpc-ecosystem/grpc-gateway/pull/155) ([mwitkow](https://github.com/mwitkow))
- Add more examples to marshalers [\#154](https://github.com/grpc-ecosystem/grpc-gateway/pull/154) ([yugui](https://github.com/yugui))
- custom marshaler: handle `Accept` headers correctly [\#152](https://github.com/grpc-ecosystem/grpc-gateway/pull/152) ([tamird](https://github.com/tamird))
- Simplify custom marshaler API [\#151](https://github.com/grpc-ecosystem/grpc-gateway/pull/151) ([yugui](https://github.com/yugui))
- Fix camel case path parameter handling in swagger [\#149](https://github.com/grpc-ecosystem/grpc-gateway/pull/149) ([yugui](https://github.com/yugui))
- Swagger dot in path template [\#148](https://github.com/grpc-ecosystem/grpc-gateway/pull/148) ([yugui](https://github.com/yugui))
- Support map types in swagger generator [\#147](https://github.com/grpc-ecosystem/grpc-gateway/pull/147) ([yugui](https://github.com/yugui))
- Cleanup custom marshaler [\#146](https://github.com/grpc-ecosystem/grpc-gateway/pull/146) ([yugui](https://github.com/yugui))
- Implement custom Marshaler support, add jsonpb implemention. [\#144](https://github.com/grpc-ecosystem/grpc-gateway/pull/144) ([tmc](https://github.com/tmc))
- Allow period in path URL templates when generating Swagger templates. [\#143](https://github.com/grpc-ecosystem/grpc-gateway/pull/143) ([ivucica](https://github.com/ivucica))
- Link to LICENSE.txt [\#142](https://github.com/grpc-ecosystem/grpc-gateway/pull/142) ([sunkuet02](https://github.com/sunkuet02))
- Support map types in swagger generator [\#141](https://github.com/grpc-ecosystem/grpc-gateway/pull/141) ([t-yuki](https://github.com/t-yuki))
- Conditionally stops checking if generated file are up-to-date [\#136](https://github.com/grpc-ecosystem/grpc-gateway/pull/136) ([yugui](https://github.com/yugui))
- Generate Swagger description for service methods using proto comments. [\#134](https://github.com/grpc-ecosystem/grpc-gateway/pull/134) ([ivucica](https://github.com/ivucica))
- Swagger definitions now have `type` set to `object`. [\#133](https://github.com/grpc-ecosystem/grpc-gateway/pull/133) ([ivucica](https://github.com/ivucica))
- go\_package option as go import path [\#129](https://github.com/grpc-ecosystem/grpc-gateway/pull/129) ([kazegusuri](https://github.com/kazegusuri))
- Fix govet errors [\#126](https://github.com/grpc-ecosystem/grpc-gateway/pull/126) ([yugui](https://github.com/yugui))
- Fix data-race in generated codes [\#125](https://github.com/grpc-ecosystem/grpc-gateway/pull/125) ([yugui](https://github.com/yugui))
- Fix \#119 - CloseNotify race with ServeHTTP [\#120](https://github.com/grpc-ecosystem/grpc-gateway/pull/120) ([cuongdo](https://github.com/cuongdo))
- Replace glog with grpclog [\#118](https://github.com/grpc-ecosystem/grpc-gateway/pull/118) ([cuongdo](https://github.com/cuongdo))
- Fix a goroutine-leak in HTTP keep-alive [\#116](https://github.com/grpc-ecosystem/grpc-gateway/pull/116) ([yugui](https://github.com/yugui))
- Fix camel case path parameter handling in swagger [\#114](https://github.com/grpc-ecosystem/grpc-gateway/pull/114) ([t-yuki](https://github.com/t-yuki))
- gofmt -s [\#112](https://github.com/grpc-ecosystem/grpc-gateway/pull/112) ([shawnps](https://github.com/shawnps))
- fix typo [\#111](https://github.com/grpc-ecosystem/grpc-gateway/pull/111) ([shawnps](https://github.com/shawnps))
- fix typo [\#110](https://github.com/grpc-ecosystem/grpc-gateway/pull/110) ([shawnps](https://github.com/shawnps))
- fixes missing swagger operation objects [\#109](https://github.com/grpc-ecosystem/grpc-gateway/pull/109) ([t-yuki](https://github.com/t-yuki))
- Add parser and swagger support for enum, no gengateway yet [\#108](https://github.com/grpc-ecosystem/grpc-gateway/pull/108) ([t-yuki](https://github.com/t-yuki))
- README: add protoc-gen-swagger too [\#105](https://github.com/grpc-ecosystem/grpc-gateway/pull/105) ([philips](https://github.com/philips))
- README: Suggest go get -u by default. [\#104](https://github.com/grpc-ecosystem/grpc-gateway/pull/104) ([dmitshur](https://github.com/dmitshur))
- Cancel context when HTTP connection is closed [\#102](https://github.com/grpc-ecosystem/grpc-gateway/pull/102) ([floridoo](https://github.com/floridoo))
- wait test server up [\#100](https://github.com/grpc-ecosystem/grpc-gateway/pull/100) ([kazegusuri](https://github.com/kazegusuri))
- Fix the swagger section of the README.md [\#98](https://github.com/grpc-ecosystem/grpc-gateway/pull/98) ([naibaf0](https://github.com/naibaf0))
- Add documentation for using Swagger [\#97](https://github.com/grpc-ecosystem/grpc-gateway/pull/97) ([achew22](https://github.com/achew22))
- Better compatibility to field names generated by protoc-gen-go [\#96](https://github.com/grpc-ecosystem/grpc-gateway/pull/96) ([yugui](https://github.com/yugui))
- Update protoc from 3.0.0-beta1 to 3.0.0-beta2 [\#95](https://github.com/grpc-ecosystem/grpc-gateway/pull/95) ([yugui](https://github.com/yugui))
- Better grpc error strings [\#94](https://github.com/grpc-ecosystem/grpc-gateway/pull/94) ([floridoo](https://github.com/floridoo))
- make available header and trailer metadata [\#93](https://github.com/grpc-ecosystem/grpc-gateway/pull/93) ([kazegusuri](https://github.com/kazegusuri))
- make grpc.DialOption configurable [\#89](https://github.com/grpc-ecosystem/grpc-gateway/pull/89) ([kazegusuri](https://github.com/kazegusuri))
- Add request in error handlers [\#88](https://github.com/grpc-ecosystem/grpc-gateway/pull/88) ([daniellowtw](https://github.com/daniellowtw))
- Improve PascalFromSnake behavior [\#85](https://github.com/grpc-ecosystem/grpc-gateway/pull/85) ([tmc](https://github.com/tmc))
- Typo grcp -\> grpc [\#81](https://github.com/grpc-ecosystem/grpc-gateway/pull/81) ([daniellowtw](https://github.com/daniellowtw))
- Add abstraction of code generator implementation [\#78](https://github.com/grpc-ecosystem/grpc-gateway/pull/78) ([yugui](https://github.com/yugui))
- Support multivalue of metadata [\#77](https://github.com/grpc-ecosystem/grpc-gateway/pull/77) ([yugui](https://github.com/yugui))
- Fix broken test [\#76](https://github.com/grpc-ecosystem/grpc-gateway/pull/76) ([yugui](https://github.com/yugui))
- Added missing instruction line in README  [\#75](https://github.com/grpc-ecosystem/grpc-gateway/pull/75) ([betrcode](https://github.com/betrcode))
- Fix a complie error in generated go files [\#71](https://github.com/grpc-ecosystem/grpc-gateway/pull/71) ([yugui](https://github.com/yugui))
- Update generated .pb.go files in third\_party [\#69](https://github.com/grpc-ecosystem/grpc-gateway/pull/69) ([pedgeio](https://github.com/pedgeio))
- Bugfix/handling headers for `Authorization` and `Host` [\#65](https://github.com/grpc-ecosystem/grpc-gateway/pull/65) ([mwitkow](https://github.com/mwitkow))
- Fix `error` field always in chunk response [\#64](https://github.com/grpc-ecosystem/grpc-gateway/pull/64) ([mwitkow](https://github.com/mwitkow))
- Update .pb.go to latest version. [\#63](https://github.com/grpc-ecosystem/grpc-gateway/pull/63) ([johansja](https://github.com/johansja))
- Run more tests in Travis CI [\#60](https://github.com/grpc-ecosystem/grpc-gateway/pull/60) ([yugui](https://github.com/yugui))
- Added http error code and error status for responseStreamChunk error [\#59](https://github.com/grpc-ecosystem/grpc-gateway/pull/59) ([kdima](https://github.com/kdima))
- Fix parsing of verb and final path component. [\#55](https://github.com/grpc-ecosystem/grpc-gateway/pull/55) ([hbchai](https://github.com/hbchai))
- add grpc.WithInsecure\(\) as option for grpc.Dial call in template [\#52](https://github.com/grpc-ecosystem/grpc-gateway/pull/52) ([pedgeio](https://github.com/pedgeio))
- update .pb.go files for latest golang proto generation [\#51](https://github.com/grpc-ecosystem/grpc-gateway/pull/51) ([pedgeio](https://github.com/pedgeio))
- Fix a build error with the latest protoc-gen-go [\#50](https://github.com/grpc-ecosystem/grpc-gateway/pull/50) ([yugui](https://github.com/yugui))
- Configure Travis CI [\#49](https://github.com/grpc-ecosystem/grpc-gateway/pull/49) ([yugui](https://github.com/yugui))
- Follow a change of go package name convention in protoc-gen-go [\#48](https://github.com/grpc-ecosystem/grpc-gateway/pull/48) ([yugui](https://github.com/yugui))
- Consider tail segments after deep wildcard [\#47](https://github.com/grpc-ecosystem/grpc-gateway/pull/47) ([yugui](https://github.com/yugui))
- Fix typo in README [\#45](https://github.com/grpc-ecosystem/grpc-gateway/pull/45) ([jonboulle](https://github.com/jonboulle))
- Fix undefined variable error in generated codes [\#42](https://github.com/grpc-ecosystem/grpc-gateway/pull/42) ([yugui](https://github.com/yugui))
- Follow changes in protoc-gen-go and grpc-go [\#41](https://github.com/grpc-ecosystem/grpc-gateway/pull/41) ([yugui](https://github.com/yugui))
- Fixes \#4 [\#40](https://github.com/grpc-ecosystem/grpc-gateway/pull/40) ([AmandaCameron](https://github.com/AmandaCameron))
- fix examples to work with go1.5 [\#39](https://github.com/grpc-ecosystem/grpc-gateway/pull/39) ([tmc](https://github.com/tmc))
- rename internal to utilties for 1.5 compatibility [\#38](https://github.com/grpc-ecosystem/grpc-gateway/pull/38) ([tmc](https://github.com/tmc))
- Reflection fix of proto3 nested messages. [\#34](https://github.com/grpc-ecosystem/grpc-gateway/pull/34) ([mwitkow](https://github.com/mwitkow))
- \[Experimental\] Make the response forwarder function customizable [\#31](https://github.com/grpc-ecosystem/grpc-gateway/pull/31) ([yugui](https://github.com/yugui))
- Add f.Flush\(\) to runtime.ForwardResponseStream [\#30](https://github.com/grpc-ecosystem/grpc-gateway/pull/30) ([vvakame](https://github.com/vvakame))
- Format error message in JSON [\#29](https://github.com/grpc-ecosystem/grpc-gateway/pull/29) ([yugui](https://github.com/yugui))
- Update examples with HTTP header context annotation [\#28](https://github.com/grpc-ecosystem/grpc-gateway/pull/28) ([yugui](https://github.com/yugui))
- Report semantic errors in the source to protoc [\#27](https://github.com/grpc-ecosystem/grpc-gateway/pull/27) ([yugui](https://github.com/yugui))
- Add support for non-nullable nested messages. [\#21](https://github.com/grpc-ecosystem/grpc-gateway/pull/21) ([dmitshur](https://github.com/dmitshur))
- Receive GRPC metadata from HTTP headers. [\#18](https://github.com/grpc-ecosystem/grpc-gateway/pull/18) ([crast](https://github.com/crast))
- Implement detailed specs of google.api.http [\#14](https://github.com/grpc-ecosystem/grpc-gateway/pull/14) ([yugui](https://github.com/yugui))
- Configure travis CI [\#13](https://github.com/grpc-ecosystem/grpc-gateway/pull/13) ([yugui](https://github.com/yugui))
- Replace our own custom option with the one defined by Google [\#12](https://github.com/grpc-ecosystem/grpc-gateway/pull/12) ([yugui](https://github.com/yugui))
- Remove useless context setup [\#11](https://github.com/grpc-ecosystem/grpc-gateway/pull/11) ([iamqizhao](https://github.com/iamqizhao))
- Fix typo, path, missing semicolon. [\#10](https://github.com/grpc-ecosystem/grpc-gateway/pull/10) ([dmitshur](https://github.com/dmitshur))
- Use a globally unique id for the custom option [\#3](https://github.com/grpc-ecosystem/grpc-gateway/pull/3) ([yugui](https://github.com/yugui))
- implement ABitOfEverythingService [\#2](https://github.com/grpc-ecosystem/grpc-gateway/pull/2) ([mattn](https://github.com/mattn))
- support streaming API calls [\#1](https://github.com/grpc-ecosystem/grpc-gateway/pull/1) ([yugui](https://github.com/yugui))



\* *This Change Log was automatically generated by [github_changelog_generator](https://github.com/skywinder/Github-Changelog-Generator)*