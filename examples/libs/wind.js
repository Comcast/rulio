function weather(where,appid) {
    var url = "http://api.openweathermap.org/data/2.5/weather?appid=" + appid + "&units=metric"
    var body = Env.http('GET', url + "&q=" + Env.encode(where));
    var weather = JSON.parse(body);
    console.log('weather ' + weather);
    return weather
}

function wind(where,appid) {
    return weather(where, appid).wind.speed
}

function temp(where,appid) {
    return weather(where, appid).main.temp
}
