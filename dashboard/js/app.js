const Clusters = {
    template: `<div>
<h2>Cluster List</h2>
<div class="table-responsive">
    <table class="table table-striped table-sm">
        <thead>
            <tr>
                <th>Name</th>
                <th>Region</th>
                <th>ResourceGroup</th>
                <th>SubscriptionID</th>
                <th>KubernetesVersion</th>
                <th>Status</th>
            </tr>
        </thead>
        <tbody>
            <tr v-for="cluster in clusters">
                <td>{{cluster.name}}</td>
                <td>{{cluster.region}}</td>
                <td>{{cluster.resourceGroup}}</td>
                <td>{{cluster.subscriptionID}}</td>
                <td>{{cluster.kubernetesVersion}}</td>
                <td>{{cluster.status}}</td>
            </tr>
        </tbody>
    </table>
</div></div>`,
    data: function () {
        return {
            clusters: {},
        }
    },
    created: function () {
        this.fetchClusters();
        setInterval(this.fetchClusters, 10000)
    },
    methods: {
        fetchClusters: function () {
            this.$http.get("/clusters").then(result => {
                this.clusters = result.body;
            }, error => {
                console.error(error);
            });
        },
    },
}

const Events = {
    template: `<div>
<h2>Event List</h2>
<div class="table-responsive">
    <table class="table table-striped table-sm">
        <thead>
            <tr>
                <th>Name</th>
                <th>Namespace</th>
                <th>LastSeen</th>
                <th>Type</th>
                <th>Reason</th>
                <th>Kind</th>
                <th>Message</th>
            </tr>
        </thead>
        <tbody>
            <tr v-for="event in events">
                <td>{{event.name}}</td>
                <td>{{event.namespace}}</td>
                <td>{{event.lastSeen}}</td>
                <td>{{event.type}}</td>
                <td>{{event.reason}}</td>
                <td>{{event.kind}}</td>
                <td>{{event.message}}</td>
            </tr>
        </tbody>
    </table>
</div></div>`,
    data: function () {
        return {
            events: {},
        }
    },
    created: function () {
        this.fetchEvents();
        setInterval(this.fetchEvents, 60000)
    },
    methods: {
        fetchEvents: function () {
            this.$http.get("/events").then(result => {
                this.events = result.body;
            }, error => {
                console.error(error);
            });
        },
    },
}

const routes = [
    { path: '/', component: Clusters },
    { path: '/clusters', component: Clusters },
    { path: '/events', component: Events }
]

const router = new VueRouter({
    routes // short for `routes: routes`
})

const app = new Vue({
    router
}).$mount('#app')