(function() {
	$('body').on('mouseenter', '[data-toggle="tooltip"]', function( event ) {
		$(this).tooltip('show');
	}).on('mouseleave', '[data-toggle="tooltip"]', function( event ) {
		$(this).tooltip('hide');
	});

	$.fn.sparkline.defaults.bar.height = 63;
	$.fn.sparkline.defaults.bar.chartRangeMin = 0;
	$.fn.sparkline.defaults.bar.barWidth = 6;
	$.fn.sparkline.defaults.bar.barSpacing = 1;
	$.fn.sparkline.defaults.bar.tooltipClassname = 'jqstooltip';
	$.fn.sparkline.defaults.bar.tooltipOffsetX = 0;
	$.fn.sparkline.defaults.bar.tooltipFormat = $.spformat('<div class="tooltip-arrow"></div><div class="tooltip-inner">{{prefix}}{{value}} {{suffix}}</div>');
	$.fn.sparkline.defaults.bar.colorMap = $.range_map({
		'0:100': '#10a0de',
		'100:1000': '#ff8a00',
		'1000:': '#7bcc3a'
	});

	moment.relativeTimeThreshold('s', 60);
	moment.relativeTimeThreshold('m', 60);
	moment.relativeTimeThreshold('h', 24);
	moment.relativeTimeThreshold('d', 28);
	moment.relativeTimeThreshold('M', 12);

})();
