function radians (num) {
  return num * Math.PI / 180;
}

function haversine (lon1,lat1,lon2,lat2) {
  var R = 6371;
  var dLat = radians(lat2-lat1);
  var dLon = radians(lon2-lon1);
  var lat1 = radians(lat1);
  var lat2 = radians(lat2);
  var a = Math.sin(dLat/2) * Math.sin(dLat/2) + Math.sin(dLon/2) * Math.sin(dLon/2) * Math.cos(lat1) * Math.cos(lat2);
  var c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a));
  var d = R * c;
  return d;
}
