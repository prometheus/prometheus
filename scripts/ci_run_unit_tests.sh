if [ "$SANITIZERS" = "with_sanitizers" ]; then
	echo -n "Run unit test with sanitizers"
	./r --asan --unit-test
else
	echo -n "Run unit test without sanitizers"
	./r --unit-test
fi
