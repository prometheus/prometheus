package docker

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"

	"universe.dagger.io/alpine"
	"universe.dagger.io/docker"
)

dagger.#Plan & {
	actions: test: build: {
		// Test: simple docker.#Build
		simple: {
			#testValue: "hello world"

			image: docker.#Build & {
				steps: [
					alpine.#Build,
					docker.#Run & {
						command: {
							name: "sh"
							flags: "-c": "echo -n $TEST >> /test.txt"
						}
						env: TEST: #testValue
					},
				]
			}

			verify: core.#ReadFile & {
				input: image.output.rootfs
				path:  "/test.txt"
			}
			verify: contents: #testValue
		}

		// Test: docker.#Build with multiple steps
		multiSteps: {
			image: docker.#Build & {
				steps: [
					alpine.#Build,
					docker.#Run & {
						command: {
							name: "sh"
							flags: "-c": "echo -n hello > /bar.txt"
						}
					},
					docker.#Run & {
						command: {
							name: "sh"
							flags: "-c": "echo -n $(cat /bar.txt) world > /foo.txt"
						}
					},
					docker.#Run & {
						command: {
							name: "sh"
							flags: "-c": "echo -n $(cat /foo.txt) >> /test.txt"
						}
					},
				]
			}

			verify: core.#ReadFile & {
				input: image.output.rootfs
				path:  "/test.txt"
			}
			verify: contents: "hello world"
		}

		// Test: simple nesting of docker.#Build
		nested: {
			build: docker.#Build & {
				steps: [
					docker.#Build & {
						steps: [
							docker.#Pull & {
								source: "alpine"
							},
							docker.#Run & {
								command: name: "ls"
							},
						]
					},
					docker.#Run & {
						command: name: "ls"
					},
				]
			}
		}

		// Test: copy respects workdir like a Dockerfile COPY
		copyWorkdir: {
			testFile: core.#Source & {
				path: "./testdata"
				include: ["Dockerfile"]
			}

			build: docker.#Build & {
				steps: [
					alpine.#Build,
					docker.#Set & {
						config: workdir: "/opt"
					},
					docker.#Copy & {
						contents: testFile.output
					},
				]
			}

			buildNoWorkdir: docker.#Build & {
				steps: [
					alpine.#Build,
					docker.#Copy & {
						contents: testFile.output
					},
				]
			}

			buildWithAbsolute: docker.#Build & {
				steps: [
					alpine.#Build,
					docker.#Set & {
						config: workdir: "/opt"
					},
					docker.#Copy & {
						contents: testFile.output
						dest:     "/somenewpath"
					},
				]
			}

			verify: core.#ReadFile & {
				input: build.output.rootfs
				path:  "/opt/Dockerfile"
			}

			verifyNoWorkDir: core.#ReadFile & {
				input: buildNoWorkdir.output.rootfs
				path:  "/Dockerfile"
			}

			verifyWithAbsolutePath: core.#ReadFile & {
				input: buildWithAbsolute.output.rootfs
				path:  "/somenewpath/Dockerfile"
			}
		}

		// Test: nested docker.#Build with 3+ levels of depth
		// FIXME: this test currently fails.
		nestedDeep: {
			//   build: docker.#Build & {
			//    steps: [
			//     docker.#Build & {
			//      steps: [
			//       docker.#Build & {
			//        steps: [
			//         docker.#Pull & {
			//          source: "alpine"
			//         },
			//         docker.#Run & {
			//          command: name: "ls"
			//         },
			//        ]
			//       },
			//       docker.#Run & {
			//        command: name: "ls"
			//       },
			//      ]
			//     },
			//     docker.#Run & {
			//      command: name: "ls"
			//     },
			//    ]
			//   }
		}
	}
}
