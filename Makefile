.PHONY: dist test clean all

TARGETS = \
	dist/rediscomp 

SRCS_OTHER = \
	$(wildcard */*.go) \
	$(wildcard *.go)

SRCS_REDISCOMP = \
	$(wildcard cmd/*/*.go)

all: $(TARGETS)
	@echo "$@ done."

clean:
	/bin/rm -f $(TARGETS)
	@echo "$@ done."

dist/rediscomp: $(SRCS_REDISCOMP) $(SRCS_OTHER)
	if [ ! -d dist ];then mkdir dist; fi
	go build -o $@ -ldflags "-X main.version=`git show -s --format=%H`" $(SRCS_REDISCOMP) 
	@echo "$@ done."
