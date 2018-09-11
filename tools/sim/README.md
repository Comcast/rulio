# Traffic generator

## Summary

This code generates rules, facts, and events for a specified number of
locations ("accounts").

The data is specified by two primary input files:

1. Device type specifications (example at `devicetypes1.js`) that
   state what events different device types emit.

2. Rules templates (example at `template1.js`) that specify (sets of)
   rules along with parameters required to instantiate those rules.

An optional configuration file (example at `config1.js`) specifies
substitutions in the template file.

When the program starts, it generates accounts, and it writes facts
and rules for those accounts to a rules engine endpoint (talking the
primitive API).  Then the program starts sending events from the
devices in the accounts.  The events are generated on a per-device,
variable schedule.  The device type specifications determine the
schedules.

The schedules and other data can be randomized according to four
distributions

1. Uniform
2. Choices (you specify what with associated probabilities)
3. Normal
4. Zipf
5. TimestampMillis (reports the current time)
6. UUID (generates one)
7. Short Ids (generates one)

for data to be `Generate`d.  See `devicetypes1.js` for `Generate`
examples.

As it runs, the program reports events per second, mean latency, and
worst latency every few seconds.

## Usage

```Shell
$ ./sim -h
Usage of ./sim:
  -accounts=10: Number of accounts to simulate
  -config="": Optional filename for configuration params
  -duration=60: Run for this many seconds
  -engine="http://localhost:9001": URL for engine endpoint
  -speed=50: Multiplier to make time run faster
  -template="template1.js": Filename for rules template
  -types="devicetypes1.js": Filename for device types
```
You can start an engine with

```Shell
(cd .. && bin/startengine.sh 2>&1 | tee engine.log | grep 'light light' | grep -v console)
```

The output shows messages written by rule actions in the example
`template1.js`.

If you feel like it, you can start a mock external service that the
rule actions will call:

```Shell
(cd ../../examples && ./endpoint.py &)
```

That service endpoint does almost nothing except report requests that
it receives.

Then start the generator with

```Shell
go build && ./sim -config=config1.js -duration=120 2>&1 | tee gen.log | \
  grep -F latencies
```

You might see something like

```
latencies,1,62.000000,62,17,67,92,107,192,34
latencies,2,61.000000,61,13,24,33,36,40,8
latencies,3,55.000000,55,10,18,19,24,24,5
latencies,4,60.000000,60,12,25,28,30,39,7
latencies,5,57.000000,57,13,28,34,45,50,9
latencies,6,61.000000,61,10,32,37,46,47,10
latencies,7,52.000000,52,14,24,28,28,44,8
...
```

> `colnames(d) = c("lab","sec","hertz","count","mean","p90","p95","p99","max","dev")`

`GOMAXPROCS` is automatically set to the number of cores unless the
environment variable `GOMAXPROCS` is set.

Now use R:

```R
d <- read.csv("gen.csv", header=FALSE)
colnames(d) = c("lab","sec","hertz","count","mean","p90","p95","p99","max","dev")
library(ggplot2)
ggplot(d, aes(sec)) + 
  scale_y_log10() + 
  geom_line(aes(y = mean, colour = "mean")) + 
  geom_line(aes(y = p95, colour = "p95")) +
  geom_line(aes(y = p99, colour = "p99")) +
  geom_line(aes(y = max, colour = "max")) +
  ylab("ms") +
  theme(legend.title=element_blank()) +
  ggtitle("Event processing latencies")
```


## ToDo

1. Report error counts with other stats
