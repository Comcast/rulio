function wind(where,appid) {
    var url = "http://api.openweathermap.org/data/2.5/weather?appid=" + appid + "&units=metric"
    var body = Env.http('GET', url + "&q=" + Env.encode(where));
    var weather = JSON.parse(body);
    console.log('wind ' + weather.wind.speed);
    return weather.wind.speed;
}
