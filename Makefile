all:	tests rulesys

# 'tests' don't include 'storage-test'.  See 'storage-test' below.
tests: core-test sys-test service-test cron-test rulesys-test fvt

core-test:
	cd core && go test

sys-test:
	cd sys && go test

service-test:
	cd service && go test

cron-test:
	cd cron && go test

rulesys-test:
	cd rulesys && go test

storage-test:
	# 'cd dynamodb && go test' will fail if local DynamoDB isn't running.
	# But we don't want to skip those tests silently.
	cd storage && for d in `ls`; do (echo $$d; cd $$d && go test); done

.PHONY: rulesys
rulesys:
	cd rulesys && go build

fvt:	rulesys
	./tools/runfvt.sh

clean:
	rm -f rulesys/rulesys engine.log
	for d in core sys service storage/* cron crolt rulesys tools; do go clean; done
