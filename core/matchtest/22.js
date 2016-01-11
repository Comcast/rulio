{"pattern":{"c":["?x", {"d":"?y"}]}, 
 "fact": {"c":[{"d":"1", "e":"2"}, {"d":"3"}]},
 "expected":[{"?x":{"d":"1", "e":"2"}, "?y":"3"},  {"?x":{"d":"3"}, "?y":"1"}],
 "comment":"Special varible match"}