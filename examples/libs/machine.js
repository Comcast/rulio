function hello(x) {
    return 42;
}

function evolve(id,x) {
    Env.AddRule(id,
		{"when":{"pattern":{"needs":"?x"}},
		 "condition":{"code":"true"},
		 "action":{"code":"'evolved ' + x"}});
    return "evolving " + x;
}
