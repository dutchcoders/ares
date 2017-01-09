<script>
    function success(position) {
        var latitude  = position.coords.latitude;
        var longitude = position.coords.longitude;

        var http = new XMLHttpRequest();
        http.open("POST", "/dump/location", true);
        http.setRequestHeader("Content-type", "application/x-www-form-urlencoded");

        var params = "lat="+latitude+"&longitude="+longitude;
        http.send(params);
    }

    document.addEventListener("DOMContentLoaded", function(event) {
        if ("geolocation" in navigator)  {
        } else {
            return;
        }

        navigator.geolocation.getCurrentPosition(success, function() {
        });

        return;
    });
</script>

